package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() { gin.SetMode(gin.TestMode) }

// MarkRead's URL-param parsing is the only validation gate that runs
// before the (DB-backed) service is touched. Every other handler reads
// pagination but tolerates bad input via defaults — those don't 400.

func TestMarkRead_RejectsInvalidID(t *testing.T) {
	h := &NotificationHandler{notif: nil}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	h.MarkRead(c)
	assert.Equal(t, 400, w.Code)
}
