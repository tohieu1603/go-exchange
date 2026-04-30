package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// These tests exercise the validation gates BEFORE any wallet/audit side-effect
// fires, so we can leave wallet+bus nil. Success-path tests would require
// mocking the wallet service (out of scope for this round).

func init() { gin.SetMode(gin.TestMode) }

func newAdjustHandler(t *testing.T) *AdminWalletHandler {
	t.Helper()
	t.Setenv("ADMIN_ADJUST_MAX_AMOUNT", "100000")
	return &AdminWalletHandler{wallet: nil, bus: nil}
}

// newGin returns the context + recorder. Read recorder.Code to assert HTTP
// status — gin's test context wraps the recorder so c.Writer is not the
// recorder itself.
func newGin(method, path string, body interface{}, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	c.Request, _ = http.NewRequest(method, path, &buf)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	return c, w
}

func decodeError(w *httptest.ResponseRecorder) string {
	var resp struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error != "" {
		return resp.Error
	}
	return resp.Message
}

func TestAdjustBalance_RejectsInvalidUserID(t *testing.T) {
	h := newAdjustHandler(t)
	c, w := newGin("POST", "/", map[string]any{"amount": 1, "reason": "x"},
		gin.Params{{Key: "id", Value: "abc"}, {Key: "currency", Value: "USDT"}})
	h.AdjustBalance(c)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAdjustBalance_RejectsZeroAmount(t *testing.T) {
	h := newAdjustHandler(t)
	c, w := newGin("POST", "/", map[string]any{"amount": 0, "reason": "x"},
		gin.Params{{Key: "id", Value: "1"}, {Key: "currency", Value: "USDT"}})
	h.AdjustBalance(c)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAdjustBalance_RejectsMissingReason(t *testing.T) {
	h := newAdjustHandler(t)
	c, w := newGin("POST", "/", map[string]any{"amount": 100, "reason": "  "},
		gin.Params{{Key: "id", Value: "1"}, {Key: "currency", Value: "USDT"}})
	h.AdjustBalance(c)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAdjustBalance_RejectsExceedsCap(t *testing.T) {
	h := newAdjustHandler(t) // cap=100000
	c, w := newGin("POST", "/", map[string]any{"amount": 200001, "reason": "manual credit"},
		gin.Params{{Key: "id", Value: "1"}, {Key: "currency", Value: "USDT"}})
	h.AdjustBalance(c)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAdjustBalance_RejectsExceedsCap_NegativeSide(t *testing.T) {
	// |amount| is what's compared — negative also gated.
	h := newAdjustHandler(t)
	c, w := newGin("POST", "/", map[string]any{"amount": -150000, "reason": "claw back"},
		gin.Params{{Key: "id", Value: "1"}, {Key: "currency", Value: "USDT"}})
	h.AdjustBalance(c)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAdjustMaxAmount_DefaultAndOverride(t *testing.T) {
	_ = os.Unsetenv("ADMIN_ADJUST_MAX_AMOUNT")
	if got := adjustMaxAmount(); got != 100000 {
		t.Errorf("default cap: want 100000 got %f", got)
	}
	t.Setenv("ADMIN_ADJUST_MAX_AMOUNT", "500")
	if got := adjustMaxAmount(); got != 500 {
		t.Errorf("override cap: want 500 got %f", got)
	}
}

func TestAdjustBalanceBatch_RejectsEmpty(t *testing.T) {
	h := newAdjustHandler(t)
	c, w := newGin("POST", "/", map[string]any{"items": []any{}},
		gin.Params{{Key: "id", Value: "1"}})
	h.AdjustBalanceBatch(c)
	if w.Code != 400 {
		t.Errorf("want 400 for empty items, got %d", w.Code)
	}
}

func TestAdjustBalanceBatch_RejectsTooMany(t *testing.T) {
	h := newAdjustHandler(t)
	items := make([]map[string]any, 21)
	for i := range items {
		items[i] = map[string]any{"currency": "USDT", "amount": 1, "reason": "x"}
	}
	c, w := newGin("POST", "/", map[string]any{"items": items},
		gin.Params{{Key: "id", Value: "1"}})
	h.AdjustBalanceBatch(c)
	if w.Code != 400 {
		t.Errorf("want 400 for >20 items, got %d", w.Code)
	}
	if msg := decodeError(w); msg == "" {
		t.Error("expected error message on too-many response")
	}
}

func TestAdjustBalanceBatch_RejectsInvalidUserID(t *testing.T) {
	h := newAdjustHandler(t)
	c, w := newGin("POST", "/", map[string]any{"items": []map[string]any{{"currency": "USDT", "amount": 1, "reason": "x"}}},
		gin.Params{{Key: "id", Value: "not-a-number"}})
	h.AdjustBalanceBatch(c)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}
