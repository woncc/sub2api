//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func userOSSTestPNG() string {
	var buf bytes.Buffer
	requirePNG := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if err := png.Encode(&buf, requirePNG); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestUserOSSStorageErrorIsNotUpstreamFailover(t *testing.T) {
	err := userOSSStorageError("bind", errors.New("redis down"))
	require.True(t, IsUserOSSStorageError(err))
	require.ErrorIs(t, err, errors.Unwrap(err))
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.NotContains(t, UserOSSClientMessage, "redis")
}

func TestForwardGrokMediaPersistBindingFailureIsUserOSSError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: staticHTTPUpstream{body: `{"request_id":"vid_1","status":"pending"}`},
		cache:        &ossPendingCache{setErr: errors.New("redis down")},
	}
	account := &Account{
		ID:          1,
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	c, rec := ginOSSTestContext()
	AttachUserOSS(c, UserOSSRequest{UserID: 4, RepoID: 9, Prefix: "file/images/"})
	AttachOSSOwner(c, 4, 8)

	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"grok-imagine-video","prompt":"cat"}`), "application/json")
	require.Nil(t, result)
	require.True(t, IsUserOSSStorageError(err))
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.Zero(t, rec.Body.Len(), "create must not return the upstream task id when binding fails")
	require.NotContains(t, rec.Body.String(), "vid_1")
}

func TestForwardGrokMediaVideoRewriteFailureIsUserOSSError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newUserOSSMemoryRepo()
	oss := newUserOSSServiceForTest(repo, nil)
	view, err := oss.Create(context.Background(), 4, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", SecretAccessKey: "sk", Bucket: "photos",
		Region: "auto", Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	oss.factory = func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		return errImageStorage{err: errors.New("SecretAccessKey=sk upload denied")}, nil
	}
	oss.allowPrivateFetch = true
	videoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "video-bytes")
	}))
	t.Cleanup(videoSrv.Close)

	body := `{"request_id":"vid_1","status":"done","video":{"url":"` + videoSrv.URL + `/clip.mp4"}}`
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: staticHTTPUpstream{body: body},
		userOSS:      oss,
		cache:        &ossPendingCache{},
	}
	account := &Account{
		ID:          1,
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	c, rec := ginOSSTestContext()
	AttachUserOSS(c, UserOSSRequest{UserID: 4, RepoID: view.ID, Prefix: "file/images/"})

	result, fwdErr := svc.ForwardGrokMedia(c.Request.Context(), c, account, GrokMediaEndpointVideoStatus, "vid_1", nil, "")
	require.Nil(t, result)
	require.True(t, IsUserOSSStorageError(fwdErr))
	var failover *UpstreamFailoverError
	require.False(t, errors.As(fwdErr, &failover))
	require.Zero(t, rec.Body.Len())
}

func TestForwardSeedanceBindingAndRewriteFailuresAreUserOSSErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":             "test-key",
			"base_url":            "https://ark.example/api/v3",
			"openai_capabilities": []any{"seedance"},
		},
	}

	bindSvc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: staticHTTPUpstream{body: `{"id":"cgt-1"}`},
		cache:        &ossPendingCache{setErr: errors.New("redis down")},
	}
	c, rec := ginOSSTestContext()
	AttachUserOSS(c, UserOSSRequest{UserID: 4, RepoID: 9, Prefix: "file/images/"})
	AttachOSSOwner(c, 4, 8)
	result, err := bindSvc.ForwardSeedance(context.Background(), c, account, SeedanceEndpointCreate, "", []byte(`{"model":"doubao-seedance","content":[{"type":"text","text":"cat"}]}`))
	require.Nil(t, result)
	require.True(t, IsUserOSSStorageError(err))
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.Zero(t, rec.Body.Len())
	require.NotContains(t, rec.Body.String(), "cgt-1")

	repo := newUserOSSMemoryRepo()
	oss := newUserOSSServiceForTest(repo, nil)
	view, createErr := oss.Create(context.Background(), 4, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", SecretAccessKey: "sk", Bucket: "photos",
		Region: "auto", Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.NoError(t, createErr)
	oss.factory = func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		return errImageStorage{err: errors.New("upload denied")}, nil
	}
	oss.allowPrivateFetch = true
	videoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "video-bytes")
	}))
	t.Cleanup(videoSrv.Close)
	statusSvc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: staticHTTPUpstream{body: `{"id":"cgt-1","status":"succeeded","content":{"video_url":"` + videoSrv.URL + `/clip.mp4"}}`},
		userOSS:      oss,
	}
	statusCtx, statusRec := ginOSSTestContext()
	AttachUserOSS(statusCtx, UserOSSRequest{UserID: 4, RepoID: view.ID, Prefix: "file/images/"})
	statusResult, statusErr := statusSvc.ForwardSeedance(context.Background(), statusCtx, account, SeedanceEndpointStatus, "seedance:cgt-1", nil)
	require.Nil(t, statusResult)
	require.True(t, IsUserOSSStorageError(statusErr))
	require.False(t, errors.As(statusErr, &failover))
	require.Zero(t, statusRec.Body.Len())
}

func TestRewriteOpenAIImageFailureIsUserOSSError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newUserOSSMemoryRepo()
	oss := newUserOSSServiceForTest(repo, nil)
	view, err := oss.Create(context.Background(), 4, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", SecretAccessKey: "sk", Bucket: "photos",
		Region: "auto", Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	oss.factory = func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		return errImageStorage{err: errors.New("upload denied")}, nil
	}
	svc := &OpenAIGatewayService{userOSS: oss}
	c, _ := ginOSSTestContext()
	AttachUserOSS(c, UserOSSRequest{UserID: 4, RepoID: view.ID, Prefix: "file/images/"})
	body := []byte(`{"data":[{"b64_json":"` + userOSSTestPNG() + `"}]}`)
	_, rewriteErr := svc.rewriteUserOSSImage(c, "req-1", body)
	require.True(t, IsUserOSSStorageError(rewriteErr))
	var failover *UpstreamFailoverError
	require.False(t, errors.As(rewriteErr, &failover))
}

func TestImageTaskUserOSSFailureUsesObjectStorageError(t *testing.T) {
	store := &imageTaskMemoryStore{}
	tasks := NewImageTaskServiceWithUploader(store, nil, 0, 0)
	repo := newUserOSSMemoryRepo()
	oss := newUserOSSServiceForTest(repo, nil)
	view, err := oss.Create(context.Background(), 4, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", SecretAccessKey: "sk", Bucket: "photos",
		Region: "auto", Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	oss.factory = func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		return errImageStorage{err: errors.New("SecretAccessKey=sk")}, nil
	}
	tasks.SetUserOSS(oss)
	created, err := tasks.Create(context.Background(), ImageTaskOwner{UserID: 4, APIKeyID: 8})
	require.NoError(t, err)
	body := []byte(`{"data":[{"b64_json":"` + userOSSTestPNG() + `"}]}`)
	require.NoError(t, tasks.CompleteWithUserOSS(context.Background(), created.ID, http.StatusOK, body, UserOSSRequest{
		UserID: 4, RepoID: view.ID, Prefix: "file/images/",
	}))
	got, err := tasks.Get(context.Background(), ImageTaskOwner{UserID: 4, APIKeyID: 8}, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusFailed, got.Status)
	require.Contains(t, string(got.Error), `"type":"object_storage_error"`)
	require.Contains(t, string(got.Error), UserOSSClientMessage)
	require.NotContains(t, string(got.Error), "SecretAccessKey")
}

type staticHTTPUpstream struct {
	body   string
	status int
}

func (u staticHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	status := u.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(u.body)),
		Request:    req,
	}, nil
}

func (u staticHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type errImageStorage struct{ err error }

func (s errImageStorage) Save(context.Context, string, string, []byte) (string, error) {
	return "", s.err
}

func ginOSSTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", nil)
	return c, rec
}
