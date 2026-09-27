package handler

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// UserOSSHandler serves per-user object storage repositories.
type UserOSSHandler struct {
	oss *service.UserOSSService
}

func NewUserOSSHandler(oss *service.UserOSSService) *UserOSSHandler {
	return &UserOSSHandler{oss: oss}
}

func (h *UserOSSHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	items, err := h.oss.List(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *UserOSSHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req service.UserOSSInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	view, err := h.oss.Create(c.Request.Context(), userID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, view)
}

func (h *UserOSSHandler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := parseUserOSSID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid repository id")
		return
	}
	var req service.UserOSSInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	view, err := h.oss.Update(c.Request.Context(), userID, id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *UserOSSHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := parseUserOSSID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid repository id")
		return
	}
	if err := h.oss.Delete(c.Request.Context(), userID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *UserOSSHandler) Check(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := parseUserOSSID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid repository id")
		return
	}
	var req service.UserOSSInput
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid request: "+err.Error())
			return
		}
	}
	resolved, err := h.oss.Check(c.Request.Context(), userID, id, req)
	if err != nil && resolved == nil {
		response.ErrorFrom(c, err)
		return
	}
	body := gin.H{"ok": err == nil}
	if err != nil {
		body["message"] = err.Error()
	} else {
		body["message"] = "connection successful"
	}
	if resolved != nil {
		body["resolved"] = resolved
	}
	response.Success(c, body)
}

func currentUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return 0, false
	}
	return subject.UserID, true
}

func parseUserOSSID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, err
	}
	return id, nil
}
