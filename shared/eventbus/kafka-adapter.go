package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type KafkaBus struct {
	brokers  []string
	writers  map[string]*kafka.Writer
	writerMu sync.Mutex
	handlers map[string][]Handler
	groups   map[string][]Handler
	rdb      *redis.Client
}

func NewKafkaBus(brokers []string, rdb *redis.Client) *KafkaBus {
	return &KafkaBus{
		brokers:  brokers,
		writers:  make(map[string]*kafka.Writer),
		handlers: make(map[string][]Handler),
		groups:   make(map[string][]Handler),
		rdb:      rdb,
	}
}

// getWriter lazy-initializes a Kafka writer per topic (thread-safe).
func (kb *KafkaBus) getWriter(topic string) *kafka.Writer {
	kb.writerMu.Lock()
	defer kb.writerMu.Unlock()
	if w, ok := kb.writers[topic]; ok {
		return w
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(kb.brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{}, // partition by key (e.g., pair)
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		Async:        false, // sync to ensure delivery before response
		RequiredAcks: kafka.RequireOne,
	}
	kb.writers[topic] = w
	return w
}

// Publish sends an event to Kafka. Extracts partition key from payload if it has a GetPair() method.
func (kb *KafkaBus) Publish(ctx context.Context, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[kafka] marshal error [%s]: %v", topic, err)
		return err
	}
	msg := kafka.Message{Value: data, Time: time.Now()}
	if m, ok := payload.(interface{ GetPair() string }); ok {
		msg.Key = []byte(m.GetPair())
	}
	if err := kb.getWriter(topic).WriteMessages(ctx, msg); err != nil {
		log.Printf("[kafka] publish error [%s]: %v", topic, err)
		return err
	}
	return nil
}

// PublishWithKey sends an event to Kafka with an explicit partition key.
func (kb *KafkaBus) PublishWithKey(ctx context.Context, topic string, payload interface{}, key string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	msg := kafka.Message{Key: []byte(key), Value: data, Time: time.Now()}
	return kb.getWriter(topic).WriteMessages(ctx, msg)
}

// PublishWS uses Redis Pub/Sub for instant WebSocket fan-out (Kafka is not used for real-time broadcast).
func (kb *KafkaBus) PublishWS(ctx context.Context, channel string, data interface{}) {
	if kb.rdb == nil {
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{"channel": channel, "data": data})
	kb.rdb.Publish(ctx, "ws:broadcast", string(payload))
}

// PublishBatch sends multiple events in one Kafka round-trip.
func (kb *KafkaBus) PublishBatch(ctx context.Context, topic string, events []interface{}, key string) error {
	msgs := make([]kafka.Message, len(events))
	for i, e := range events {
		data, _ := json.Marshal(e)
		msgs[i] = kafka.Message{
			Key:   []byte(key),
			Value: data,
			Time:  time.Now(),
		}
	}
	return kb.getWriter(topic).WriteMessages(ctx, msgs...)
}

// Subscribe registers a handler (same interface as Redis Bus).
func (kb *KafkaBus) Subscribe(topic string, handler Handler) {
	kb.handlers[topic] = append(kb.handlers[topic], handler)
}

// StartConsumer creates a Kafka consumer group reader and processes messages.
// Captures current handlers snapshot so each consumer group runs its own handlers.
func (kb *KafkaBus) StartConsumer(ctx context.Context, topic, group, consumer string) {
	// Snapshot handlers registered for this topic at start time
	handlers := make([]Handler, len(kb.handlers[topic]))
	copy(handlers, kb.handlers[topic])
	// Clear topic handlers so next Subscribe+StartConsumer pair gets fresh set
	kb.handlers[topic] = nil

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        kb.brokers,
		Topic:          topic,
		GroupID:        group,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        3 * time.Second,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})

	go func() {
		defer r.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := r.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				time.Sleep(2 * time.Second)
				continue
			}
			allOK := true
			for _, h := range handlers {
				if err := h(ctx, fmt.Sprintf("%d-%d", msg.Partition, msg.Offset), msg.Value); err != nil {
					log.Printf("[kafka] handler error [%s/%s/%d-%d]: %v", group, topic, msg.Partition, msg.Offset, err)
					allOK = false
				}
			}
			if allOK {
				if err := r.CommitMessages(ctx, msg); err != nil {
					log.Printf("[kafka] commit error [%s]: %v", group, err)
				}
			}
		}
	}()

	log.Printf("[kafka] consumer started: topic=%s group=%s consumer=%s handlers=%d", topic, group, consumer, len(handlers))
}

// Close closes all writers.
func (kb *KafkaBus) Close() {
	for _, w := range kb.writers {
		w.Close()
	}
}

// CreateTopics ensures topics exist with proper partition count.
func (kb *KafkaBus) CreateTopics(topics []string, partitions int) error {
	conn, err := kafka.Dial("tcp", kb.brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	configs := make([]kafka.TopicConfig, len(topics))
	for i, t := range topics {
		configs[i] = kafka.TopicConfig{
			Topic:             t,
			NumPartitions:     partitions,
			ReplicationFactor: 1,
		}
	}
	return conn.CreateTopics(configs...)
}
