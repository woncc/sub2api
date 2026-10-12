//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// REV-001: a Seedance create with oss-id must keep the create-time billing
// model in the pending record, so the completion poll can still bill.
func TestSeedanceCreateWithUserOSSKeepsBillingModel(t *testing.T) {
	h, slots, _, upstream := newGrokMediaSlotHandler(t, false, false, service.PlatformOpenAI)
	upstream.call = func(*http.Request, int64) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"id":"task-ark","status":"queued"}`))}, nil
	}
	c, w := grokMediaSlotContext(context.Background(), true)
	key, _ := middleware.GetAPIKeyFromContext(c)
	key.Group.Platform = service.PlatformOpenAI
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	require.True(t, ok)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks",
		strings.NewReader(`{"model":"doubao-seedance","content":[{"type":"text","text":"waves"}]}`))
	c.Params = gin.Params{{Key: "task_id", Value: "task-ark"}}
	service.AttachUserOSS(c, service.UserOSSRequest{UserID: subject.UserID, RepoID: 9, Prefix: "videos/"})
	service.AttachOSSOwner(c, subject.UserID, key.ID)

	h.SeedanceTasks(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	slots.assertReleased(t)

	taskKey := service.SeedanceTaskKey("task-ark")
	pending, err := h.gatewayService.LoadGrokVideoPendingBilling(context.Background(), taskKey, subject.UserID, key.ID)
	require.NoError(t, err)
	require.NotNil(t, pending)
	require.Equal(t, "doubao-seedance", pending.Model)
	require.Equal(t, int64(9), pending.OSSRepositoryID)
	require.Equal(t, "videos/", pending.OSSPath)

	result := &service.OpenAIForwardResult{}
	result.Usage.OutputTokens = 12345
	billed := prepareSeedanceCompletionBilling(context.Background(), h, key, subject, taskKey, result)
	require.NotNil(t, billed, "completion with tokens must be billed")
	require.Equal(t, "doubao-seedance", billed.Model)
}
