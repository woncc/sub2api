package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type handlerOSSRepo struct {
	mu   sync.Mutex
	next int64
	rows map[int64]*service.UserOSSRecord
}

func (r *handlerOSSRepo) ListByUser(_ context.Context, userID int64) ([]*service.UserOSSRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*service.UserOSSRecord{}
	for _, row := range r.rows {
		if row.UserID == userID {
			copy := *row
			out = append(out, &copy)
		}
	}
	return out, nil
}

func (r *handlerOSSRepo) GetByUser(_ context.Context, userID, id int64) (*service.UserOSSRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[id]
	if row == nil || row.UserID != userID {
		return nil, service.ErrUserOSSNotFound
	}
	copy := *row
	return &copy, nil
}

func (r *handlerOSSRepo) Create(_ context.Context, rec *service.UserOSSRecord) (*service.UserOSSRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows == nil {
		r.rows = map[int64]*service.UserOSSRecord{}
	}
	r.next++
	copy := *rec
	copy.ID = r.next
	r.rows[copy.ID] = &copy
	stored := copy
	return &stored, nil
}

func (r *handlerOSSRepo) Update(context.Context, *service.UserOSSRecord) error { return nil }
func (r *handlerOSSRepo) Delete(_ context.Context, userID, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[id]
	if row == nil || row.UserID != userID {
		return service.ErrUserOSSNotFound
	}
	delete(r.rows, id)
	return nil
}

type handlerOSSEnc struct{}

func (handlerOSSEnc) Encrypt(v string) (string, error) { return "enc:" + v, nil }
func (handlerOSSEnc) Decrypt(v string) (string, error) {
	return strings.TrimPrefix(v, "enc:"), nil
}

type handlerOSSKeys struct{}

func (handlerOSSKeys) EncryptionKeyConfigured() bool { return true }

func newOSSHandler() *UserOSSHandler {
	repo := &handlerOSSRepo{}
	svc := service.NewUserOSSService(repo, handlerOSSEnc{}, handlerOSSKeys{}, func(context.Context, *config.ImageStorageConfig) (service.ImageStorage, error) {
		return ossHeadStorage{}, nil
	})
	return NewUserOSSHandler(svc)
}

type ossHeadStorage struct{}

func (ossHeadStorage) Save(context.Context, string, string, []byte) (string, error) {
	return "https://cdn.example/key", nil
}
func (ossHeadStorage) HeadBucket(context.Context) error { return nil }

func ossContext(method, target, body string, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
	return c, w
}

func TestUserOSSHandlerDoesNotReturnSecrets(t *testing.T) {
	h := newOSSHandler()
	c, w := ossContext(http.MethodPost, "/api/v1/user/oss", `{
		"provider":"aliyun",
		"access_key_id":"ak",
		"access_key_secret":"super-secret",
		"bucket":"photos",
		"region":"cn-hangzhou",
		"domain":"https://cdn.example"
	}`, 5)
	h.Create(c)
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotContains(t, w.Body.String(), "super-secret")
	require.Contains(t, w.Body.String(), `"secret_configured":true`)

	c, w = ossContext(http.MethodGet, "/api/v1/user/oss", "", 9)
	h.List(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "photos")

	c, w = ossContext(http.MethodGet, "/api/v1/user/oss", "", 5)
	h.List(c)
	require.Contains(t, w.Body.String(), "photos")
	require.NotContains(t, w.Body.String(), "super-secret")

	var payload struct {
		Data struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.NotEmpty(t, payload.Data.Items)

	c, w = ossContext(http.MethodDelete, "/api/v1/user/oss/1", "", 9)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Delete(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserOSSHandlerCheckUsesStoredSecret(t *testing.T) {
	h := newOSSHandler()
	c, w := ossContext(http.MethodPost, "/api/v1/user/oss", `{
		"provider":"s3","access_key_id":"ak","secret_access_key":"sk",
		"bucket":"b","region":"auto","endpoint":"https://s3.example","domain":"https://cdn.example"
	}`, 1)
	h.Create(c)
	require.Equal(t, http.StatusCreated, w.Code)

	c, w = ossContext(http.MethodPost, "/api/v1/user/oss/1/check", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.Check(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"ok":true`)
	require.NotContains(t, w.Body.String(), `"sk"`)
}
