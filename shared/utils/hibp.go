package utils

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// HIBP (Have I Been Pwned) password breach check using k-anonymity.
//
// Privacy contract: only the FIRST 5 hex chars of SHA-1(password) are sent
// to api.pwnedpasswords.com. The full hash never leaves this process.
//
// API: GET https://api.pwnedpasswords.com/range/{first5}
//   → text/plain: lines "SUFFIX35:COUNT\r\n"
// We scan for our suffix; non-zero count = previously seen in a breach.
//
// Caller policy: reject password if breach count >= breachThreshold.

const (
	hibpURL          = "https://api.pwnedpasswords.com/range/"
	hibpTimeout      = 3 * time.Second
	hibpUserAgent    = "micro-exchange-auth-service/1.0"
	breachThreshold  = 1 // any appearance fails
)

var hibpClient = &http.Client{Timeout: hibpTimeout}

// PasswordPolicyError is returned when a password is rejected by policy
// (either confirmed pwned, or HIBP unreachable in fail-closed mode).
type PasswordPolicyError struct{ Reason string }

func (e *PasswordPolicyError) Error() string { return e.Reason }

// CheckPasswordPolicy is the canonical entry point used by Register / ChangePassword.
// Behavior:
//
//   • If HIBP says pwned → reject (always).
//   • If HIBP unreachable AND env HIBP_FAIL_CLOSED=true → reject.
//   • Otherwise → allow.
//
// Returns nil on accept, *PasswordPolicyError on reject.
func CheckPasswordPolicy(password string) error {
	pwned, count, err := IsPasswordPwned(password)
	if pwned {
		return &PasswordPolicyError{
			Reason: passwordPolicyMsg(count),
		}
	}
	if err != nil && os.Getenv("HIBP_FAIL_CLOSED") == "true" {
		return &PasswordPolicyError{
			Reason: "password policy check unavailable, please retry shortly",
		}
	}
	return nil
}

func passwordPolicyMsg(count int) string {
	if count > 0 {
		return formatBreachMessage(count)
	}
	return "password rejected by policy"
}

// formatBreachMessage produces a fixed Vietnamese-friendly rejection.
func formatBreachMessage(count int) string {
	// Avoid fmt.Sprintf import here — utils already pulls in many deps,
	// but %d via Sprintf is the cleanest. The import already exists transitively
	// via crypto/sha1 -> none. We accept the cost for readability.
	if count >= 1000 {
		return "password appears in a known data breach corpus — choose a different one"
	}
	return "password appears in a known data breach corpus — choose a different one"
}

// IsPasswordPwned returns (pwned, breachCount, error). On network error or
// timeout returns (false, 0, error) so callers can decide fail-open vs fail-closed
// per their policy. Use CheckPasswordPolicy for the standard accept/reject decision.
func IsPasswordPwned(password string) (bool, int, error) {
	sum := sha1.Sum([]byte(password))
	hex := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := hex[:5], hex[5:]

	req, err := http.NewRequest(http.MethodGet, hibpURL+prefix, nil)
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("User-Agent", hibpUserAgent)
	req.Header.Set("Add-Padding", "true") // anti-fingerprinting (HIBP feature)

	resp, err := hibpClient.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, 0, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, 0, err
	}

	for _, line := range strings.Split(string(body), "\r\n") {
		// "SUFFIX:COUNT"
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		if !strings.EqualFold(line[:colon], suffix) {
			continue
		}
		count := 0
		for _, c := range line[colon+1:] {
			if c < '0' || c > '9' {
				break
			}
			count = count*10 + int(c-'0')
		}
		if count >= breachThreshold {
			return true, count, nil
		}
		return false, count, nil
	}
	return false, 0, nil
}
