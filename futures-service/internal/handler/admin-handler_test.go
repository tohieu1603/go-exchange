package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// We test only the URL-param parsing gates of CloseUserPosition. Reaching
// the service path requires a real DB + redis; those are integration-tested
// elsewhere.

func newAdminHandler() *AdminHandler {
	// futures + bus left nil; both gates run before either is touched.
	return &AdminHandler{futures: nil, bus: nil}
}

func newGin(method, path string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(method, path, &bytes.Buffer{})
	c.Params = params
	return c, w
}

func TestCloseUserPosition_RejectsInvalidUserID(t *testing.T) {
	h := newAdminHandler()
	c, w := newGin("POST", "/", gin.Params{
		{Key: "id", Value: "abc"},
		{Key: "positionId", Value: "1"},
	})
	h.CloseUserPosition(c)
	if w.Code != 400 {
		t.Errorf("want 400 invalid user id, got %d", w.Code)
	}
}

func TestCloseUserPosition_RejectsInvalidPositionID(t *testing.T) {
	h := newAdminHandler()
	c, w := newGin("POST", "/", gin.Params{
		{Key: "id", Value: "1"},
		{Key: "positionId", Value: "not-a-number"},
	})
	h.CloseUserPosition(c)
	if w.Code != 400 {
		t.Errorf("want 400 invalid position id, got %d", w.Code)
	}
}
