// Package http is the inbound gin adapter for the notification service.
package http

import (
	"strconv"
	"time"

	"github.com/cryptox/notification-service/internal/domain"
	"github.com/cryptox/notification-service/internal/usecase"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type notificationDTO struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"userId"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Pair      string    `json:"pair,omitempty"`
	IsRead    bool      `json:"isRead"`
	CreatedAt time.Time `json:"createdAt"`
}

func toDTOs(ns []domain.Notification) []notificationDTO {
	out := make([]notificationDTO, len(ns))
	for i, n := range ns {
		out[i] = notificationDTO{
			ID: n.ID, UserID: n.UserID, Type: n.Type, Title: n.Title,
			Message: n.Message, Pair: n.Pair, IsRead: n.IsRead, CreatedAt: n.CreatedAt,
		}
	}
	return out
}

// Handler exposes the notification HTTP endpoints.
type Handler struct {
	uc *usecase.NotificationUseCase
}

func NewHandler(uc *usecase.NotificationUseCase) *Handler { return &Handler{uc: uc} }

// List godoc
// @Summary  List notifications
// @Tags     notifications
// @Produce  json
// @Security CookieAuth
// @Success  200 {object} map[string]interface{}
// @Router   /notifications [get]
func (h *Handler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	notifs, total, err := h.uc.List(c.Request.Context(), userID, page, size, c.Query("unread") == "true")
	if err != nil {
		response.Error(c, 500, "failed to load notifications")
		return
	}
	response.Page(c, toDTOs(notifs), total, page, size)
}

// UnreadCount godoc
// @Summary  Unread notification count
// @Tags     notifications
// @Produce  json
// @Security CookieAuth
// @Success  200 {object} map[string]interface{}
// @Router   /notifications/unread-count [get]
func (h *Handler) UnreadCount(c *gin.Context) {
	count, err := h.uc.UnreadCount(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.Error(c, 500, "failed to count notifications")
		return
	}
	response.OK(c, map[string]int64{"count": count})
}

// MarkRead godoc
// @Summary  Mark a notification as read
// @Tags     notifications
// @Produce  json
// @Security CookieAuth
// @Param    id path int true "Notification ID"
// @Success  200 {object} map[string]interface{}
// @Router   /notifications/{id}/read [post]
func (h *Handler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid id")
		return
	}
	if err := h.uc.MarkRead(c.Request.Context(), middleware.GetUserID(c), uint(id)); err != nil {
		response.Error(c, 500, "failed to mark read")
		return
	}
	response.OK(c, nil)
}

// MarkAllRead godoc
// @Summary  Mark all notifications as read
// @Tags     notifications
// @Produce  json
// @Security CookieAuth
// @Success  200 {object} map[string]interface{}
// @Router   /notifications/read-all [post]
func (h *Handler) MarkAllRead(c *gin.Context) {
	if err := h.uc.MarkAllRead(c.Request.Context(), middleware.GetUserID(c)); err != nil {
		response.Error(c, 500, "failed to mark all read")
		return
	}
	response.OK(c, nil)
}
