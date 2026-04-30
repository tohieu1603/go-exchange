package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// URL-param validation gates only — service-layer paths need a real DB.

func newAdminHandler() *AdminHandler {
	return &AdminHandler{orders: nil, bus: nil}
}

func newGin(method, path string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(method, path, &bytes.Buffer{})
	c.Params = params
	return c, w
}

func TestCancelUserOrder_RejectsInvalidUserID(t *testing.T) {
	h := newAdminHandler()
	c, w := newGin("POST", "/", gin.Params{
		{Key: "id", Value: "xx"},
		{Key: "orderId", Value: "1"},
	})
	h.CancelUserOrder(c)
	if w.Code != 400 {
		t.Errorf("want 400 invalid user id, got %d", w.Code)
	}
}

func TestCancelUserOrder_RejectsInvalidOrderID(t *testing.T) {
	h := newAdminHandler()
	c, w := newGin("POST", "/", gin.Params{
		{Key: "id", Value: "1"},
		{Key: "orderId", Value: "not-int"},
	})
	h.CancelUserOrder(c)
	if w.Code != 400 {
		t.Errorf("want 400 invalid order id, got %d", w.Code)
	}
}

func TestUserOrders_RejectsInvalidUserID(t *testing.T) {
	h := newAdminHandler()
	c, w := newGin("GET", "/", gin.Params{{Key: "id", Value: "abc"}})
	h.UserOrders(c)
	if w.Code != 400 {
		t.Errorf("want 400 invalid user id, got %d", w.Code)
	}
}
