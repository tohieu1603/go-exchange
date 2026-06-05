package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryptox/notification-service/internal/domain"
	"github.com/cryptox/notification-service/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// fakeRepo is an in-memory NotificationRepository. When failWith is set every
// method returns it, letting tests assert the transport hides internal errors.
type fakeRepo struct {
	items    []domain.Notification
	failWith error
}

func (r *fakeRepo) Create(_ context.Context, n *domain.Notification) error {
	if r.failWith != nil {
		return r.failWith
	}
	r.items = append(r.items, *n)
	return nil
}

func (r *fakeRepo) FindByUser(_ context.Context, userID uint, unreadOnly bool, _, _ int) ([]domain.Notification, int64, error) {
	if r.failWith != nil {
		return nil, 0, r.failWith
	}
	var out []domain.Notification
	for _, n := range r.items {
		if n.UserID == userID && (!unreadOnly || !n.IsRead) {
			out = append(out, n)
		}
	}
	return out, int64(len(out)), nil
}

func (r *fakeRepo) UnreadCount(_ context.Context, userID uint) (int64, error) {
	if r.failWith != nil {
		return 0, r.failWith
	}
	var c int64
	for _, n := range r.items {
		if n.UserID == userID && !n.IsRead {
			c++
		}
	}
	return c, nil
}

func (r *fakeRepo) MarkAsRead(_ context.Context, userID, id uint) error {
	if r.failWith != nil {
		return r.failWith
	}
	for i := range r.items {
		if r.items[i].UserID == userID && r.items[i].ID == id {
			r.items[i].IsRead = true
		}
	}
	return nil
}

func (r *fakeRepo) MarkAllRead(_ context.Context, userID uint) error {
	if r.failWith != nil {
		return r.failWith
	}
	for i := range r.items {
		if r.items[i].UserID == userID {
			r.items[i].IsRead = true
		}
	}
	return nil
}

type noopBroadcaster struct{}

func (noopBroadcaster) Broadcast(string, interface{}) {}

// newRouter wires the real handler over the real use case + the supplied repo,
// injecting a fixed authenticated user (what JWTAuth would set in production).
func newRouter(repo domain.NotificationRepository, userID uint) *gin.Engine {
	uc := usecase.NewNotificationUseCase(repo, noopBroadcaster{})
	h := NewHandler(uc)
	r := gin.New()
	grp := r.Group("/api/notifications", func(c *gin.Context) { c.Set("userId", userID); c.Next() })
	grp.GET("", h.List)
	grp.GET("/unread-count", h.UnreadCount)
	grp.POST("/read-all", h.MarkAllRead)
	grp.POST("/:id/read", h.MarkRead)
	return r
}

func do(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestList_ReturnsUsersNotifications(t *testing.T) {
	repo := &fakeRepo{items: []domain.Notification{
		{ID: 1, UserID: 7, Type: "trade", Title: "Filled", Message: "BTC order filled"},
		{ID: 2, UserID: 9, Type: "trade", Title: "Other", Message: "not yours"},
	}}
	w := do(newRouter(repo, 7), http.MethodGet, "/api/notifications")
	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	assert.EqualValues(t, 1, data["totalElements"], "only the caller's notifications are returned")
}

func TestUnreadCount_OK(t *testing.T) {
	repo := &fakeRepo{items: []domain.Notification{
		{ID: 1, UserID: 7, IsRead: false},
		{ID: 2, UserID: 7, IsRead: true},
	}}
	w := do(newRouter(repo, 7), http.MethodGet, "/api/notifications/unread-count")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"count":1`)
}

func TestMarkRead_InvalidIDIsClientError(t *testing.T) {
	w := do(newRouter(&fakeRepo{}, 7), http.MethodPost, "/api/notifications/not-a-number/read")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid id")
}

func TestList_RepoFailureIsHiddenAs500(t *testing.T) {
	repo := &fakeRepo{failWith: errors.New("pq: connection refused host=10.0.0.5 password=secret")}
	w := do(newRouter(repo, 7), http.MethodGet, "/api/notifications")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to load notifications")
	assert.NotContains(t, w.Body.String(), "10.0.0.5", "infra detail must not leak to the API")
	assert.NotContains(t, w.Body.String(), "secret")
}

func TestMarkAllRead_RepoFailureIsHiddenAs500(t *testing.T) {
	repo := &fakeRepo{failWith: errors.New("deadlock detected on table notifications")}
	w := do(newRouter(repo, 7), http.MethodPost, "/api/notifications/read-all")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "deadlock", "DB detail must not leak")
}
