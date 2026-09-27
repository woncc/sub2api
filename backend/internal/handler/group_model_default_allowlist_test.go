package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImagesOmittedModelIsCheckedAgainstGroupAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(&service.ConcurrencyService{}, SSEPingFormatNone, 0),
	}
	group := &service.Group{
		Platform:             service.PlatformOpenAI,
		AllowImageGeneration: true,
		ModelAllowlist: service.GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"gpt-5.4"},
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"draw"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, Group: group})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3, Concurrency: 1})

	handler.Images(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "gpt-image-2")
	require.Contains(t, recorder.Body.String(), "model_not_found")
	require.Contains(t, recorder.Body.String(), "not available for this group")
}

func TestGrokRealtimeOmittedModelIsCheckedAgainstGroupAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(&service.ConcurrencyService{}, SSEPingFormatNone, 0),
	}
	group := &service.Group{
		Platform: service.PlatformGrok,
		ModelAllowlist: service.GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"grok-4"},
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "Upgrade")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, Group: group})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3, Concurrency: 1})

	handler.GrokRealtime(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "grok-voice-latest")
	require.Contains(t, recorder.Body.String(), "model_not_found")
}
