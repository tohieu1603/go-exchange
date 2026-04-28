package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client represents a single WebSocket connection.
type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	subs   map[string]bool
	userID uint // authenticated user ID (0 = anonymous)
	mu     sync.RWMutex
}

// privateChannelPrefixes are channels that require matching userID.
var privateChannelPrefixes = []string{"notification@", "position@", "liquidation@"}

func isPrivateChannel(channel string) bool {
	for _, p := range privateChannelPrefixes {
		if len(channel) > len(p) && channel[:len(p)] == p {
			return true
		}
	}
	return false
}

func (c *Client) canAccessChannel(channel string) bool {
	if c.userID == 0 {
		return false
	}
	// Channel format: "prefix@{userID}" — extract and compare
	for _, p := range privateChannelPrefixes {
		if len(channel) > len(p) && channel[:len(p)] == p {
			idStr := channel[len(p):]
			var id uint
			for _, ch := range idStr {
				if ch < '0' || ch > '9' {
					return false
				}
				id = id*10 + uint(ch-'0')
			}
			return id == c.userID
		}
	}
	return false
}

// Message is the WebSocket broadcast payload.
type Message struct {
	Channel string      `json:"channel"`
	Data    interface{} `json:"data"`
}

type SubRequest struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
}

// Hub manages local WS connections + subscribes to Redis Pub/Sub for cross-instance broadcast.
// Architecture:
//
//	Instance A: Broadcast("trades@BTC_USDT", data)
//	  -> Redis PUBLISH "ws:broadcast" {channel, data}
//	  -> All instances (A, B, C) receive via SUBSCRIBE
//	  -> Each instance fans out to its local clients
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	localBcast chan Message
	rdb        *redis.Client
	jwtSecret  string
	mu         sync.RWMutex
}

func NewHub(rdb *redis.Client, jwtSecret ...string) *Hub {
	h := &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		localBcast: make(chan Message, 1024),
	}
	if rdb != nil {
		h.rdb = rdb
	}
	if len(jwtSecret) > 0 {
		h.jwtSecret = jwtSecret[0]
	}
	return h
}

// Run starts the hub event loop: handles register/unregister + local fan-out.
func (h *Hub) Run() {
	// If Redis available, subscribe to cross-instance broadcasts
	if h.rdb != nil {
		go h.redisSubscriber()
	}

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case msg := <-h.localBcast:
			h.fanOut(msg)
		}
	}
}

// Broadcast publishes to Redis Pub/Sub (multi-instance) or local channel (single-instance).
func (h *Hub) Broadcast(channel string, data interface{}) {
	if h.rdb != nil {
		payload, _ := json.Marshal(Message{Channel: channel, Data: data})
		if err := h.rdb.Publish(context.Background(), "ws:broadcast", string(payload)).Err(); err != nil {
			// Fallback to local if Redis fails
			h.localBcast <- Message{Channel: channel, Data: data}
		}
	} else {
		// Single-instance: direct local fan-out
		h.localBcast <- Message{Channel: channel, Data: data}
	}
}

// redisSubscriber listens for broadcasts from all instances via Redis Pub/Sub.
func (h *Hub) redisSubscriber() {
	ctx := context.Background()
	sub := h.rdb.Subscribe(ctx, "ws:broadcast")
	defer sub.Close()

	ch := sub.Channel()
	for msg := range ch {
		var wsMsg Message
		if err := json.Unmarshal([]byte(msg.Payload), &wsMsg); err != nil {
			continue
		}
		h.fanOut(wsMsg)
	}
}

// fanOut sends message to all locally connected clients subscribed to the channel.
func (h *Hub) fanOut(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		client.mu.RLock()
		subscribed := client.subs[msg.Channel]
		client.mu.RUnlock()

		if !subscribed {
			continue
		}

		select {
		case client.send <- data:
		default:
			// Client buffer full - disconnect slow client (non-blocking to avoid goroutine leak)
			select {
			case h.unregister <- client:
			default:
			}
		}
	}
}

// HandleWS upgrades HTTP to WebSocket.
// Optional auth: pass ?token=JWT to authenticate for private channels.
func (h *Hub) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}

	var userID uint
	if tokenStr := c.Query("token"); tokenStr != "" && h.jwtSecret != "" {
		userID = parseUserIDFromJWT(tokenStr, h.jwtSecret)
	}

	client := &Client{
		conn:   conn,
		send:   make(chan []byte, 256),
		subs:   make(map[string]bool),
		userID: userID,
	}
	h.register <- client

	go client.writePump()
	go client.readPump(h)
}

// parseUserIDFromJWT extracts userID from a JWT token (best-effort, no error on failure).
func parseUserIDFromJWT(tokenStr, secret string) uint {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return 0
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0
	}
	if uid, ok := claims["userId"].(float64); ok {
		return uint(uid)
	}
	return 0
}

func (c *Client) readPump(hub *Hub) {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var req SubRequest
		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}

		c.mu.Lock()
		switch req.Action {
		case "subscribe":
			// Block subscription to private channels (notification@, position@, liquidation@)
			// unless channel matches this client's authenticated userID
			if isPrivateChannel(req.Channel) && !c.canAccessChannel(req.Channel) {
				c.mu.Unlock()
				continue
			}
			c.subs[req.Channel] = true
		case "unsubscribe":
			delete(c.subs, req.Channel)
		}
		c.mu.Unlock()
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

// ClientCount returns current connected client count (for monitoring).
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
