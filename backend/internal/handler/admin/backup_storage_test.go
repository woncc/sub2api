//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBackupStorageTestResponsesIncludeResolved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &memSettings{values: map[string]string{}}
	backup := service.NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}, prefixEncryptor{}, func(context.Context, *service.BackupS3Config) (service.BackupObjectStore, error) {
		return headOnlyBackupStore{}, nil
	}, nil)
	images := service.NewImageStorageSettingService(repo, prefixEncryptor{}, backup, func(context.Context, *config.ImageStorageConfig) (service.ImageStorage, error) {
		return headOnlyImageStore{}, nil
	}, config.ImageStorageConfig{})
	h := NewBackupHandler(backup, nil, images)

	t.Run("s3 config success", func(t *testing.T) {
		body := postTest(t, h.TestS3Connection, `{
			"provider":"aliyun_oss",
			"region":"cn-hangzhou",
			"bucket":"example",
			"access_key_id":"AK",
			"secret_access_key":"sekrit",
			"force_path_style":true
		}`)
		require.Equal(t, true, body["ok"])
		require.Equal(t, "connection successful", body["message"])
		require.NotContains(t, mustJSON(t, body), "sekrit")
		resolved := body["resolved"].(map[string]any)
		require.Equal(t, "https://s3.oss-cn-hangzhou.aliyuncs.com", resolved["endpoint"])
		require.Equal(t, "cn-hangzhou", resolved["region"])
		require.Equal(t, false, resolved["force_path_style"])
	})

	t.Run("s3 config failure omits unresolved and secrets", func(t *testing.T) {
		body := postTest(t, h.TestS3Connection, `{
			"provider":"nope",
			"bucket":"example",
			"access_key_id":"AK",
			"secret_access_key":"sekrit"
		}`)
		require.Equal(t, false, body["ok"])
		require.Contains(t, body["message"], "unknown storage provider")
		require.NotContains(t, body, "resolved")
		require.NotContains(t, mustJSON(t, body), "sekrit")
	})

	t.Run("image storage success", func(t *testing.T) {
		body := postTest(t, h.TestImageStorageConnection, `{
			"enabled":true,
			"provider":"qiniu",
			"region":"cn-east-1",
			"bucket":"space",
			"access_key_id":"AK",
			"secret_access_key":"sekrit"
		}`)
		require.Equal(t, true, body["ok"])
		require.Equal(t, "connection successful", body["message"])
		require.NotContains(t, mustJSON(t, body), "sekrit")
		resolved := body["resolved"].(map[string]any)
		require.Equal(t, "https://s3.cn-east-1.qiniucs.com", resolved["endpoint"])
		require.Equal(t, "cn-east-1", resolved["region"])
		require.Equal(t, false, resolved["force_path_style"])
	})
}

func postTest(t *testing.T, handler func(*gin.Context), payload string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	handler(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	return envelope.Data
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

type memSettings struct {
	values map[string]string
}

func (m *memSettings) Get(context.Context, string) (*service.Setting, error) { return nil, nil }
func (m *memSettings) GetValue(_ context.Context, key string) (string, error) {
	return m.values[key], nil
}
func (m *memSettings) Set(_ context.Context, key, value string) error {
	m.values[key] = value
	return nil
}
func (m *memSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (m *memSettings) SetMultiple(context.Context, map[string]string) error { return nil }
func (m *memSettings) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (m *memSettings) Delete(context.Context, string) error { return nil }

type prefixEncryptor struct{}

func (prefixEncryptor) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (prefixEncryptor) Decrypt(ciphertext string) (string, error) {
	rest, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", io.ErrUnexpectedEOF
	}
	return rest, nil
}

type headOnlyBackupStore struct{}

func (headOnlyBackupStore) Upload(context.Context, string, io.Reader, string) (int64, error) {
	return 0, nil
}
func (headOnlyBackupStore) UploadFile(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (headOnlyBackupStore) Download(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (headOnlyBackupStore) Delete(context.Context, string) error { return nil }
func (headOnlyBackupStore) PresignURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (headOnlyBackupStore) HeadBucket(context.Context) error { return nil }

type headOnlyImageStore struct{}

func (headOnlyImageStore) Save(context.Context, string, string, []byte) (string, error) {
	return "", nil
}
func (headOnlyImageStore) HeadBucket(context.Context) error { return nil }
