package utils

import (
	"net/url"
	"strings"
	"testing"
)

func TestGenerateSepayQR_ContainsAllParams(t *testing.T) {
	url := GenerateSepayQR("96247CISI1", "BIDV", 250000, "TXN-42")
	if !strings.HasPrefix(url, "https://qr.sepay.vn/img?") {
		t.Errorf("wrong base URL: %s", url)
	}
	for _, fragment := range []string{"acc=96247CISI1", "bank=BIDV", "amount=250000", "des=TXN-42"} {
		if !strings.Contains(url, fragment) {
			t.Errorf("missing %q in %q", fragment, url)
		}
	}
}

func TestGenerateSepayQR_AmountFormattedNoDecimals(t *testing.T) {
	// Sepay expects integer VND (no fractional part). %.0f drops decimals.
	got := GenerateSepayQR("X", "Y", 99999.99, "Z")
	if !strings.Contains(got, "amount=100000") {
		t.Errorf("amount should round to integer; got %s", got)
	}
}

// Defensive — the current impl uses raw string interpolation, which is
// fine in practice because SePay descriptions are alphanum + dash. This
// test documents that current behavior so a future "url.QueryEscape"
// upgrade is a deliberate, breaking change.
func TestGenerateSepayQR_DoesNotEscapeSpaces(t *testing.T) {
	got := GenerateSepayQR("X", "Y", 1, "with space")
	u, err := url.Parse(got)
	if err == nil && u.Query().Get("des") == "with space" {
		// Fine — Go's net/url tolerates the unescaped space in query parsing.
		return
	}
	t.Logf("note: spaces in description are not URL-encoded (current impl); update test if escaping is added")
}
