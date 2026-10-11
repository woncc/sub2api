//go:build unit

package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokMediaUserOSSBindingFailureDoesNotBlameAccount(t *testing.T) {
	h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
	h.gatewayService.EnableOpenAIAccountSchedulerForTest()
	bindings.setErr = errors.New("redis down")
	upstream.call = func(*http.Request, int64) (*http.Response, error) {
		return grokMediaJSONResponse(`{"request_id":"vid_oss","status":"pending"}`), nil
	}
	c, w := grokMediaSlotContext(context.Background(), true)
	service.AttachUserOSS(c, service.UserOSSRequest{UserID: 10, RepoID: 9, Prefix: "file/images/"})
	service.AttachOSSOwner(c, 10, 20)

	h.GrokVideoGeneration(c)

	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Equal(t, "object_storage_error", gjson.Get(w.Body.String(), "error.type").String())
	require.Equal(t, service.UserOSSClientMessage, gjson.Get(w.Body.String(), "error.message").String())
	require.NotContains(t, w.Body.String(), "vid_oss")
	require.NotContains(t, w.Body.String(), "redis down")
	require.Equal(t, 1, upstream.calls, "object storage failure must not fail over")
	slots.assertReleased(t)
	assertGrokMediaAccountNotBlamed(t, h)
}

func TestGrokMediaUserOSSVideoRewriteFailureDoesNotBlameAccount(t *testing.T) {
	h, slots, _, upstream := newGrokMediaSlotHandler(t, false, false)
	h.gatewayService.EnableOpenAIAccountSchedulerForTest()
	h.gatewayService.SetUserOSS(newFailingHandlerUserOSS())
	upstream.call = func(*http.Request, int64) (*http.Response, error) {
		return grokMediaJSONResponse(`{"request_id":"task","status":"done","video":{"url":"https://cdn.example/clip.mp4"}}`), nil
	}
	c, w := grokMediaSlotContext(context.Background(), false)
	service.AttachUserOSS(c, service.UserOSSRequest{UserID: 10, RepoID: 9, Prefix: "file/images/"})

	h.GrokVideoStatus(c)

	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Equal(t, "object_storage_error", gjson.Get(w.Body.String(), "error.type").String())
	require.Equal(t, service.UserOSSClientMessage, gjson.Get(w.Body.String(), "error.message").String())
	require.NotContains(t, w.Body.String(), "SecretAccessKey")
	require.NotContains(t, w.Body.String(), "topsecret")
	require.Equal(t, 1, upstream.calls)
	slots.assertReleased(t)
	assertGrokMediaAccountNotBlamed(t, h)
}

func assertGrokMediaAccountNotBlamed(t *testing.T, h *OpenAIGatewayHandler) {
	t.Helper()
	reported := 0
	for id := int64(1); id <= 3; id++ {
		rate, ok := h.gatewayService.OpenAIAccountErrorRateForTest(id)
		if !ok {
			continue
		}
		reported++
		require.Zero(t, rate, "account %d was scored as a failure", id)
	}
	require.Equal(t, 1, reported, "the successful upstream account must be reported once")
}

func grokMediaJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type handlerUserOSSRepo struct {
	row *service.UserOSSRecord
}

func (r handlerUserOSSRepo) ListByUser(context.Context, int64) ([]*service.UserOSSRecord, error) {
	return nil, nil
}
func (r handlerUserOSSRepo) GetByUser(_ context.Context, userID, id int64) (*service.UserOSSRecord, error) {
	if r.row == nil || r.row.UserID != userID || r.row.ID != id {
		return nil, service.ErrUserOSSNotFound
	}
	copy := *r.row
	return &copy, nil
}
func (r handlerUserOSSRepo) Create(context.Context, *service.UserOSSRecord) (*service.UserOSSRecord, error) {
	return nil, errors.New("unused")
}
func (r handlerUserOSSRepo) Update(context.Context, *service.UserOSSRecord) error {
	return errors.New("unused")
}
func (r handlerUserOSSRepo) Delete(context.Context, int64, int64) error { return errors.New("unused") }

type handlerUserOSSEncryptor struct{}

func (handlerUserOSSEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}
func (handlerUserOSSEncryptor) Decrypt(ciphertext string) (string, error) {
	plain, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", errors.New("ciphertext is not decryptable")
	}
	return plain, nil
}

type handlerUserOSSKeys struct{}

func (handlerUserOSSKeys) EncryptionKeyConfigured() bool { return true }

type handlerErrStorage struct{ err error }

func (s handlerErrStorage) Save(context.Context, string, string, []byte) (string, error) {
	return "", s.err
}

func newFailingHandlerUserOSS() *service.UserOSSService {
	return service.NewUserOSSService(
		handlerUserOSSRepo{row: &service.UserOSSRecord{
			ID: 9, UserID: 10, Provider: "s3", Bucket: "photos", Region: "auto",
			Endpoint: "https://s3.example", Domain: "https://cdn.example",
			AccessKeyID: "ak", SecretEncrypted: "enc:topsecret",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
		handlerUserOSSEncryptor{},
		handlerUserOSSKeys{},
		func(context.Context, *config.ImageStorageConfig) (service.ImageStorage, error) {
			return nil, errors.New("SecretAccessKey=topsecret")
		},
	)
}
