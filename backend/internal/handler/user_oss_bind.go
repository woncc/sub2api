package handler

import (
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) SetUserOSS(oss *service.UserOSSService) {
	if h != nil {
		h.userOSS = oss
	}
}

// bindGatewayUserOSS resolves oss-id for this API key's user.
// A request that already carries a binding (the async image worker) is left unchanged.
// deferOffload keeps the synchronous writer from uploading; completion uploads instead.
func (h *OpenAIGatewayHandler) bindGatewayUserOSS(c *gin.Context, userID int64, deferOffload bool) error {
	if c == nil || c.Request == nil {
		return nil
	}
	if _, ok := service.UserOSSRequestFromContext(c.Request.Context()); ok {
		return nil
	}
	ossID := strings.TrimSpace(c.GetHeader("oss-id"))
	if ossID == "" {
		return nil
	}
	if h == nil || h.userOSS == nil || userID <= 0 {
		return infraerrors.ServiceUnavailable("USER_OSS_UNAVAILABLE", "object storage is unavailable")
	}
	spec, err := h.userOSS.SpecFromHeaders(c.Request.Context(), userID, ossID, c.GetHeader("oss-path"))
	if err != nil {
		return err
	}
	if spec == nil {
		return nil
	}
	spec.Defer = deferOffload
	service.AttachUserOSS(c, *spec)
	return nil
}

func (h *OpenAIGatewayHandler) rejectUserOSS(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	if status < 400 {
		status = http.StatusBadRequest
	}
	message := infraerrors.Message(err)
	if message == "" && err != nil {
		message = err.Error()
	}
	h.errorResponse(c, status, "invalid_request_error", message)
}
