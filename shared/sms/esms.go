package sms

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ESMSSender — Vietnam local SMS provider (esms.vn).
//
// Why offer this alongside Twilio:
//   • Twilio: global, expensive for Vietnamese numbers (often $0.02-0.05/SMS)
//   • eSMS: Vietnam-domestic provider, ~₫500-₫700/SMS for OTP, supports brand name
//
// Pick via env: SMS_PROVIDER=esms (otherwise twilio is default).
//
// Required env when SMS_PROVIDER=esms:
//
//	ESMS_API_KEY        — issued by eSMS dashboard
//	ESMS_SECRET_KEY     — issued by eSMS dashboard
//	ESMS_BRANDNAME      — registered brand (defaults to "Baotrips" for testing)
//	ESMS_SMS_TYPE       — "2" (CSKH/marketing) | "8" (OTP) — default 8
//
// API spec: https://developer.esms.vn/apis/sms-brand-name
//
// The /MainService.svc/json/SendMultipleMessage_V4_post_json endpoint takes:
//
//	{ApiKey, SecretKey, Phone, Content, Brandname, SmsType, IsUnicode}
type ESMSSender struct {
	apiKey    string
	secretKey string
	brandName string
	smsType   string
	client    *http.Client
}

// NewESMSSender constructs from explicit creds (for tests / DI).
func NewESMSSender(apiKey, secretKey, brandName, smsType string) *ESMSSender {
	if smsType == "" {
		smsType = "8"
	}
	return &ESMSSender{
		apiKey: apiKey, secretKey: secretKey,
		brandName: brandName, smsType: smsType,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Send delivers an OTP via eSMS. Phone is normalized to local format
// (84xxxxxxxxx) — eSMS rejects international "+84" prefix.
func (s *ESMSSender) Send(phone, message string) error {
	body := map[string]string{
		"ApiKey":    s.apiKey,
		"SecretKey": s.secretKey,
		"Phone":     normalizeVNPhone(phone),
		"Content":   message,
		"Brandname": s.brandName,
		"SmsType":   s.smsType,
		"IsUnicode": "0", // 0 = ASCII (more chars per segment), 1 = Unicode (Vietnamese diacritics)
	}
	buf, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost,
		"https://rest.esms.vn/MainService.svc/json/SendMultipleMessage_V4_post_json/",
		strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("esms http %d: %s", resp.StatusCode, string(respBody))
	}

	// eSMS embeds business-level status in JSON: CodeResult "100" = success.
	var parsed struct {
		CodeResult     string `json:"CodeResult"`
		ErrorMessage   string `json:"ErrorMessage"`
		SMSID          string `json:"SMSID"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return fmt.Errorf("esms decode: %w (body=%s)", err, string(respBody))
	}
	if parsed.CodeResult != "100" {
		return fmt.Errorf("esms code=%s msg=%s", parsed.CodeResult, parsed.ErrorMessage)
	}
	return nil
}

// normalizeVNPhone converts "+84..." or "0..." to the "84..." form eSMS expects.
// Pass-through for other formats (caller's responsibility).
func normalizeVNPhone(p string) string {
	p = strings.TrimSpace(p)
	switch {
	case strings.HasPrefix(p, "+84"):
		return "84" + p[3:]
	case strings.HasPrefix(p, "0"):
		return "84" + p[1:]
	}
	return p
}

// urlEncode is provided in case future extensions need x-www-form-urlencoded.
// Currently unused by JSON path above.
var _ = url.QueryEscape
