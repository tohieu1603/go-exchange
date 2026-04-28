package handler

import (
	"strconv"

	"github.com/cryptox/notification-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type NotificationHandler struct {
	notif *service.NotificationService
}

func NewNotificationHandler(notif *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{notif: notif}
}

// List godoc
// @Summary      List notifications
// @Tags         notifications
// @Produce      json
// @Security     CookieAuth
// @Param        page    query  int   false "Page (default 1)"
// @Param        size    query  int   false "Page size (default 20, max 50)"
// @Param        unread  query  bool  false "Only unread notifications"
// @Success      200  {object}  map[string]interface{}
// @Router       /notifications [get]
func (h *NotificationHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	unreadOnly := c.Query("unread") == "true"
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}

	notifs, total, err := h.notif.GetUserNotifications(userID, page, size, unreadOnly)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, notifs, total, page, size)
}

// UnreadCount godoc
// @Summary      Get unread notification count
// @Tags         notifications
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  map[string]interface{}
// @Router       /notifications/unread-count [get]
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID := middleware.GetUserID(c)
	count := h.notif.UnreadCount(userID)
	response.OK(c, map[string]int64{"count": count})
}

// MarkRead godoc
// @Summary      Mark a notification as read
// @Tags         notifications
// @Produce      json
// @Security     CookieAuth
// @Param        id  path  int  true  "Notification ID"
// @Success      200  {object}  map[string]interface{}
// @Router       /notifications/{id}/read [post]
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid id")
		return
	}
	h.notif.MarkAsRead(userID, uint(id))
	response.OK(c, nil)
}

// MarkAllRead godoc
// @Summary      Mark all notifications as read
// @Tags         notifications
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  map[string]interface{}
// @Router       /notifications/read-all [post]
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	h.notif.MarkAllRead(userID)
	response.OK(c, nil)
}
