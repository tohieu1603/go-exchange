// Package elastic is the Elasticsearch adapter implementing application.Indexer.
package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/elastic/go-elasticsearch/v8"
)

// Client wraps the Elasticsearch client with index helpers.
type Client struct {
	es *elasticsearch.Client
}

func New(addresses []string) (*Client, error) {
	es, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: addresses})
	if err != nil {
		return nil, fmt.Errorf("es client: %w", err)
	}
	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("es connect: %w", err)
	}
	defer res.Body.Close()
	log.Println("[elasticsearch] connected")
	return &Client{es: es}, nil
}

// EnsureIndex creates the index with the given mapping if it does not exist.
func (c *Client) EnsureIndex(name string, mapping map[string]interface{}) {
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

// Index writes a single document (implements application.Indexer).
func (c *Client) Index(ctx context.Context, index, id string, doc interface{}) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	res, err := c.es.Index(index, bytes.NewReader(body), c.es.Index.WithDocumentID(id), c.es.Index.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("es index: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("es index response: %s", res.Status())
	}
	return nil
}

// IndexMappings declares the field mappings per document type.
var IndexMappings = map[string]map[string]interface{}{
	"trades": {"mappings": map[string]interface{}{"properties": map[string]interface{}{
		"pair": map[string]string{"type": "keyword"}, "buyerId": map[string]string{"type": "long"},
		"sellerId": map[string]string{"type": "long"}, "price": map[string]string{"type": "double"},
		"amount": map[string]string{"type": "double"}, "total": map[string]string{"type": "double"},
		"side": map[string]string{"type": "keyword"}, "createdAt": map[string]string{"type": "date"},
	}}},
	"orders": {"mappings": map[string]interface{}{"properties": map[string]interface{}{
		"orderId": map[string]string{"type": "long"}, "userId": map[string]string{"type": "long"},
		"pair": map[string]string{"type": "keyword"}, "side": map[string]string{"type": "keyword"},
		"type": map[string]string{"type": "keyword"}, "status": map[string]string{"type": "keyword"},
		"price": map[string]string{"type": "double"}, "amount": map[string]string{"type": "double"},
		"filledAmount": map[string]string{"type": "double"}, "updatedAt": map[string]string{"type": "date"},
	}}},
	"balances": {"mappings": map[string]interface{}{"properties": map[string]interface{}{
		"userId": map[string]string{"type": "long"}, "currency": map[string]string{"type": "keyword"},
		"delta": map[string]string{"type": "double"}, "reason": map[string]string{"type": "keyword"},
		"createdAt": map[string]string{"type": "date"},
	}}},
	"notifications": {"mappings": map[string]interface{}{"properties": map[string]interface{}{
		"userId": map[string]string{"type": "long"}, "type": map[string]string{"type": "keyword"},
		"title": map[string]string{"type": "text"}, "message": map[string]string{"type": "text"},
		"pair": map[string]string{"type": "keyword"}, "createdAt": map[string]string{"type": "date"},
	}}},
}
