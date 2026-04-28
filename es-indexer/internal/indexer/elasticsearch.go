package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

// ESClient wraps Elasticsearch client with index helpers
type ESClient struct {
	es *elasticsearch.Client
}

func NewESClient(addresses []string) (*ESClient, error) {
	cfg := elasticsearch.Config{
		Addresses: addresses,
		Transport: nil,
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("es client error: %w", err)
	}
	// Verify connection
	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("es connect error: %w", err)
	}
	defer res.Body.Close()
	log.Println("[elasticsearch] connected")
	return &ESClient{es: es}, nil
}

// EnsureIndex creates index with mapping if not exists
func (c *ESClient) EnsureIndex(name string, mapping map[string]interface{}) {
	res, err := c.es.Indices.Exists([]string{name})
	if err == nil && res.StatusCode == 200 {
		res.Body.Close()
		return
	}
	if res != nil {
		res.Body.Close()
	}

	body, _ := json.Marshal(mapping)
	createRes, err := c.es.Indices.Create(name, c.es.Indices.Create.WithBody(bytes.NewReader(body)))
	if err != nil {
		log.Printf("[elasticsearch] create index %s error: %v", name, err)
		return
	}
	defer createRes.Body.Close()
	log.Printf("[elasticsearch] index %s created", name)
}

// Index documents a single record
func (c *ESClient) Index(ctx context.Context, index string, id string, doc interface{}) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	res, err := c.es.Index(index, bytes.NewReader(body),
		c.es.Index.WithDocumentID(id),
		c.es.Index.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("es index error: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("es index response error: %s", res.Status())
	}
	return nil
}

// Index mappings for all document types
var IndexMappings = map[string]map[string]interface{}{
	"trades": {
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"pair":      map[string]string{"type": "keyword"},
				"buyerId":   map[string]string{"type": "long"},
				"sellerId":  map[string]string{"type": "long"},
				"price":     map[string]string{"type": "double"},
				"amount":    map[string]string{"type": "double"},
				"total":     map[string]string{"type": "double"},
				"side":      map[string]string{"type": "keyword"},
				"createdAt": map[string]string{"type": "date"},
			},
		},
	},
	"orders": {
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"orderId":      map[string]string{"type": "long"},
				"userId":       map[string]string{"type": "long"},
				"pair":         map[string]string{"type": "keyword"},
				"side":         map[string]string{"type": "keyword"},
				"type":         map[string]string{"type": "keyword"},
				"status":       map[string]string{"type": "keyword"},
				"price":        map[string]string{"type": "double"},
				"amount":       map[string]string{"type": "double"},
				"filledAmount": map[string]string{"type": "double"},
				"updatedAt":    map[string]string{"type": "date"},
			},
		},
	},
	"balances": {
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"userId":    map[string]string{"type": "long"},
				"currency":  map[string]string{"type": "keyword"},
				"delta":     map[string]string{"type": "double"},
				"reason":    map[string]string{"type": "keyword"},
				"createdAt": map[string]string{"type": "date"},
			},
		},
	},
	"notifications": {
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"userId":    map[string]string{"type": "long"},
				"type":      map[string]string{"type": "keyword"},
				"title":     map[string]string{"type": "text"},
				"message":   map[string]string{"type": "text"},
				"pair":      map[string]string{"type": "keyword"},
				"createdAt": map[string]string{"type": "date"},
			},
		},
	},
}

// TradeDoc for ES indexing
type TradeDoc struct {
	Pair      string    `json:"pair"`
	BuyerID   uint      `json:"buyerId"`
	SellerID  uint      `json:"sellerId"`
	Price     float64   `json:"price"`
	Amount    float64   `json:"amount"`
	Total     float64   `json:"total"`
	Side      string    `json:"side"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrderDoc for ES indexing
type OrderDoc struct {
	OrderID      uint      `json:"orderId"`
	UserID       uint      `json:"userId"`
	Pair         string    `json:"pair"`
	Side         string    `json:"side"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Price        float64   `json:"price"`
	Amount       float64   `json:"amount"`
	FilledAmount float64   `json:"filledAmount"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// BalanceDoc for ES audit trail
type BalanceDoc struct {
	UserID    uint      `json:"userId"`
	Currency  string    `json:"currency"`
	Delta     float64   `json:"delta"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

// NotificationDoc for ES indexing
type NotificationDoc struct {
	UserID    uint      `json:"userId"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Pair      string    `json:"pair"`
	CreatedAt time.Time `json:"createdAt"`
}

// AuditDoc shape for the audit_logs ES index.
// `Timestamp` is the canonical time field — Kibana uses it as the index
// pattern's @timestamp surrogate when creating the data view.
type AuditDoc struct {
	UserID    uint      `json:"userId"`
	Email     string    `json:"email,omitempty"`
	Action    string    `json:"action"`
	Outcome   string    `json:"outcome"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
	DeviceID  string    `json:"deviceId,omitempty"`
	NewDevice bool      `json:"newDevice,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Timestamp time.Time `json:"@timestamp"`
}
