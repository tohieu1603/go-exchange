// Package sms sends transactional SMS via Twilio REST API.
//
// Configuration via env:
//
//	TWILIO_ACCOUNT_SID
//	TWILIO_AUTH_TOKEN
//	TWILIO_FROM       — sender phone (E.164, e.g. "+12025551234")
//
// If any required var is empty, NewFromEnv returns a NoopSender that logs
// instead of sending — useful for local dev without a Twilio account.
package sms

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Sender abstracts SMS dispatch so callers can pass NoopSender in tests/dev.
type Sender interface {
	Send(toE164, message string) error
}

// NewFromEnv selects an SMS provider based on env:
//
//	SMS_PROVIDER=esms      → ESMSSender (Vietnam local — cheaper for VN numbers)
//	SMS_PROVIDER=twilio    → TwilioSender (default)
//	(provider unset)       → TwilioSender if TWILIO_* set, else NoopSender
//
// Returns NoopSender if the chosen provider's credentials are incomplete —
// avoids hard failure during dev but logs the OTP to stdout.
func NewFromEnv() Sender {
	switch os.Getenv("SMS_PROVIDER") {
	case "esms":
		key := os.Getenv("ESMS_API_KEY")
		sec := os.Getenv("ESMS_SECRET_KEY")
		brand := os.Getenv("ESMS_BRANDNAME")
		if key == "" || sec == "" || brand == "" {
			return &NoopSender{}
		}
		return NewESMSSender(key, sec, brand, os.Getenv("ESMS_SMS_TYPE"))
	}
	// Default / SMS_PROVIDER=twilio
	sid := os.Getenv("TWILIO_ACCOUNT_SID")
	tok := os.Getenv("TWILIO_AUTH_TOKEN")
	from := os.Getenv("TWILIO_FROM")
	if sid == "" || tok == "" || from == "" {
		return &NoopSender{}
	}
	return &TwilioSender{sid: sid, token: tok, from: from,
		client: &http.Client{Timeout: 5 * time.Second}}
}

// TwilioSender posts to https://api.twilio.com/2010-04-01/Accounts/{SID}/Messages.json
// with HTTP Basic auth.
type TwilioSender struct {
	sid    string
	token  string
	from   string
	client *http.Client
}

func (s *TwilioSender) Send(to, message string) error {
	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", s.sid)
	form := url.Values{}
	form.Set("From", s.from)
	form.Set("To", to)
	form.Set("Body", message)

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.sid, s.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio http %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// NoopSender logs the SMS payload to stdout — for dev without Twilio.
type NoopSender struct{}

func (NoopSender) Send(to, message string) error {
	log.Printf("[sms:noop] to=%s message=%q", to, message)
	return nil
}

// VerifyCodeMessage builds the standard Vietnamese OTP body.
func VerifyCodeMessage(code string) string {
	return fmt.Sprintf(
		"[Micro-Exchange] Mã xác thực thiết bị mới: %s. Có hiệu lực 5 phút. Không chia sẻ mã này.",
		code,
	)
}
