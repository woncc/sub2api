package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardRejectsImageGenerationToolModelOutsideAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{Group: &Group{
		Platform:             PlatformOpenAI,
		AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"gpt-5.4"},
		},
	}})
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation","model":"gpt-image-2"}]}`)

	result, err := (&OpenAIGatewayService{}).Forward(context.Background(), c, account, body)

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":"model_not_found"`)
	require.Contains(t, recorder.Body.String(), "gpt-image-2")
}

func TestImageGenerationToolModelAllowlistAllowsListedModelAndSkipsGrok(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"gpt-image-2"}]}`)
	group := &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4", "gpt-image-2"}}}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{Group: group})
	require.NoError(t, rejectDisallowedImageGenerationToolModel(c, &Account{Platform: PlatformOpenAI}, body))
	require.Zero(t, recorder.Body.Len())

	wildcard := &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*"}}}
	c.Set("api_key", &APIKey{Group: wildcard})
	require.NoError(t, rejectDisallowedImageGenerationToolModel(c, &Account{Platform: PlatformOpenAI}, body))

	restricted := &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4"}}}
	c.Set("api_key", &APIKey{Group: restricted})
	require.NoError(t, rejectDisallowedImageGenerationToolModel(c, &Account{Platform: PlatformGrok}, body))
}
