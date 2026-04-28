package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ESAnomalyClient queries Elasticsearch for richer anomaly aggregations.
// Optional — anomaly detector continues to work in-process even if ES is
// unreachable. When connected, it powers two extra checks:
//
//   - Cross-IP burst: same user logged in from N+ distinct IPs in the past
//                     1 hour (signals shared/leaked credentials).
//   - Failure-burst from one IP: many failed logins for many emails from
//                                 same IP — IP-level credential stuffing.
//
// All queries hit the "audit_logs" index with a tight time filter to bound cost.
type ESAnomalyClient struct {
	url    string
	client *http.Client
}

func NewESAnomalyClient(url string) *ESAnomalyClient {
	if url == "" {
		return nil
	}
	return &ESAnomalyClient{
		url:    url,
		client: &http.Client{Timeout: 2 * time.Second},
	}
}

// DistinctIPCountForUser returns how many distinct IPs the user logged in
// successfully from in the past `window`. Returns -1 on error (caller decides).
func (c *ESAnomalyClient) DistinctIPCountForUser(ctx context.Context, userID uint, window time.Duration) int {
	if c == nil {
		return -1
	}
	body := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []interface{}{
					map[string]interface{}{"term": map[string]interface{}{"userId": userID}},
					map[string]interface{}{"term": map[string]interface{}{"action": "login.success"}},
					map[string]interface{}{"range": map[string]interface{}{
						"@timestamp": map[string]interface{}{
							"gte": fmt.Sprintf("now-%ds", int(window.Seconds())),
						},
					}},
				},
			},
		},
		"aggs": map[string]interface{}{
			"distinct_ips": map[string]interface{}{
				"cardinality": map[string]interface{}{"field": "ip.keyword"},
			},
		},
	}
	var resp struct {
		Aggregations struct {
			DistinctIPs struct {
				Value int `json:"value"`
			} `json:"distinct_ips"`
		} `json:"aggregations"`
	}
	if err := c.searchInto(ctx, "audit_logs", body, &resp); err != nil {
		return -1
	}
	return resp.Aggregations.DistinctIPs.Value
}

// FailureBurstFromIP counts distinct emails that login.failure from the IP
// in the past `window`. High value = credential stuffing.
func (c *ESAnomalyClient) FailureBurstFromIP(ctx context.Context, ip string, window time.Duration) int {
	if c == nil || ip == "" {
		return -1
	}
	body := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []interface{}{
					map[string]interface{}{"term": map[string]interface{}{"ip.keyword": ip}},
					map[string]interface{}{"term": map[string]interface{}{"action": "login.failure"}},
					map[string]interface{}{"range": map[string]interface{}{
						"@timestamp": map[string]interface{}{
							"gte": fmt.Sprintf("now-%ds", int(window.Seconds())),
						},
					}},
				},
			},
		},
		"aggs": map[string]interface{}{
			"distinct_emails": map[string]interface{}{
				"cardinality": map[string]interface{}{"field": "email.keyword"},
			},
		},
	}
	var resp struct {
		Aggregations struct {
			DistinctEmails struct {
				Value int `json:"value"`
			} `json:"distinct_emails"`
		} `json:"aggregations"`
	}
	if err := c.searchInto(ctx, "audit_logs", body, &resp); err != nil {
		return -1
	}
	return resp.Aggregations.DistinctEmails.Value
}

// searchInto runs a search query and decodes the JSON into out.
func (c *ESAnomalyClient) searchInto(ctx context.Context, index string, body, out interface{}) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.url+"/"+index+"/_search", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("es status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
