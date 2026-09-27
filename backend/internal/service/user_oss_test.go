package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type userOSSMemoryRepo struct {
	mu      sync.Mutex
	next    int64
	rows    map[int64]*UserOSSRecord
	getErr  error
	updates int
}

func newUserOSSMemoryRepo() *userOSSMemoryRepo {
	return &userOSSMemoryRepo{rows: map[int64]*UserOSSRecord{}}
}

func (r *userOSSMemoryRepo) ListByUser(_ context.Context, userID int64) ([]*UserOSSRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*UserOSSRecord, 0)
	for _, row := range r.rows {
		if row.UserID == userID {
			copy := *row
			out = append(out, &copy)
		}
	}
	return out, nil
}

func (r *userOSSMemoryRepo) GetByUser(_ context.Context, userID, id int64) (*UserOSSRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return nil, r.getErr
	}
	row, ok := r.rows[id]
	if !ok || row.UserID != userID {
		return nil, ErrUserOSSNotFound
	}
	copy := *row
	return &copy, nil
}

func (r *userOSSMemoryRepo) Create(_ context.Context, rec *UserOSSRecord) (*UserOSSRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	copy := *rec
	copy.ID = r.next
	r.rows[copy.ID] = &copy
	stored := copy
	return &stored, nil
}

func (r *userOSSMemoryRepo) Update(_ context.Context, rec *UserOSSRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[rec.ID]
	if !ok || row.UserID != rec.UserID {
		return ErrUserOSSNotFound
	}
	copy := *rec
	r.rows[rec.ID] = &copy
	r.updates++
	return nil
}

func (r *userOSSMemoryRepo) Delete(_ context.Context, userID, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[id]
	if !ok || row.UserID != userID {
		return ErrUserOSSNotFound
	}
	delete(r.rows, id)
	return nil
}

type userOSSEncryptor struct{}

func (userOSSEncryptor) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (userOSSEncryptor) Decrypt(ciphertext string) (string, error) {
	plain, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", errors.New("ciphertext is not decryptable")
	}
	return plain, nil
}

type userOSSStorage struct {
	bucket string
	saved  []savedImage
}

func (s *userOSSStorage) Save(_ context.Context, key, contentType string, data []byte) (string, error) {
	s.saved = append(s.saved, savedImage{key: key, contentType: contentType, data: data})
	return "https://cdn.example/" + key, nil
}

func (s *userOSSStorage) HeadBucket(context.Context) error { return nil }

func newUserOSSServiceForTest(repo *userOSSMemoryRepo, storages *[]*userOSSStorage) *UserOSSService {
	return NewUserOSSService(repo, userOSSEncryptor{}, &BackupService{encryptionKeyConfigured: true}, func(_ context.Context, cfg *config.ImageStorageConfig) (ImageStorage, error) {
		storage := &userOSSStorage{bucket: cfg.Bucket}
		if storages != nil {
			*storages = append(*storages, storage)
		}
		if cfg.PublicBaseURL != "" {
			storage.saved = nil
		}
		return &userOSSPublicStorage{userOSSStorage: storage, base: cfg.PublicBaseURL}, nil
	})
}

type userOSSPublicStorage struct {
	*userOSSStorage
	base string
}

func (s *userOSSPublicStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	if _, err := s.userOSSStorage.Save(ctx, key, contentType, data); err != nil {
		return "", err
	}
	if s.base != "" {
		return strings.TrimRight(s.base, "/") + "/" + key, nil
	}
	return "https://signed.example/" + key, nil
}

func TestUserOSSCreateHidesSecretAndIsolatesUsers(t *testing.T) {
	repo := newUserOSSMemoryRepo()
	svc := newUserOSSServiceForTest(repo, nil)
	view, err := svc.Create(context.Background(), 7, UserOSSInput{
		Provider:        " aliyun ",
		AccessKeyID:     " ak ",
		AccessKeySecret: " super-secret ",
		Bucket:          " photos ",
		Region:          " cn-hangzhou ",
		Domain:          "https://cdn.example/",
	})
	require.NoError(t, err)
	require.True(t, view.SecretConfigured)
	require.Equal(t, UserOSSProviderAliyun, view.Provider)
	require.Equal(t, "photos", view.Bucket)
	require.Equal(t, "https://cdn.example", view.Domain)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "super-secret")
	require.NotContains(t, string(raw), "access_key_secret")

	_, err = svc.List(context.Background(), 8)
	require.NoError(t, err)
	require.Empty(t, mustList(t, svc, 8))
	_, err = svc.SpecFromHeaders(context.Background(), 8, "1", "")
	require.ErrorIs(t, err, ErrUserOSSNotFound)
	spec, err := svc.SpecFromHeaders(context.Background(), 7, "1", "file/images")
	require.NoError(t, err)
	require.Equal(t, "file/images/", spec.Prefix)
}

func TestUserOSSUpdatePreservesSecretAndFailsClosed(t *testing.T) {
	repo := newUserOSSMemoryRepo()
	svc := newUserOSSServiceForTest(repo, nil)
	created, err := svc.Create(context.Background(), 1, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", SecretAccessKey: "topsecret",
		Bucket: "b", Region: "auto", Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	updated, err := svc.Update(context.Background(), 1, created.ID, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", Bucket: "b2", Region: "auto",
		Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	require.Equal(t, "b2", updated.Bucket)
	require.True(t, updated.SecretConfigured)
	stored, err := repo.GetByUser(context.Background(), 1, created.ID)
	require.NoError(t, err)
	require.Equal(t, "enc:topsecret", stored.SecretEncrypted)

	repo.getErr = errors.New("database unavailable")
	_, err = svc.Update(context.Background(), 1, created.ID, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", SecretAccessKey: "", Bucket: "wiped",
		Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.Error(t, err)
	require.Equal(t, 1, repo.updates)
	repo.getErr = nil
	stored, err = repo.GetByUser(context.Background(), 1, created.ID)
	require.NoError(t, err)
	require.Equal(t, "b2", stored.Bucket)
	require.Equal(t, "enc:topsecret", stored.SecretEncrypted)
}

func TestUserOSSProviderMappingAndCheck(t *testing.T) {
	repo := newUserOSSMemoryRepo()
	var storages []*userOSSStorage
	svc := newUserOSSServiceForTest(repo, &storages)
	view, err := svc.Create(context.Background(), 3, UserOSSInput{
		Provider: "tencent", SecretID: "sid", SecretKey: "skey",
		BucketURL: "https://example-1250000000.cos.ap-guangzhou.myqcloud.com",
		Domain:    "https://cdn.example",
	})
	require.NoError(t, err)
	require.Equal(t, "example-1250000000", view.Bucket)
	require.Equal(t, "ap-guangzhou", view.Region)

	cf, err := svc.Create(context.Background(), 3, UserOSSInput{
		Provider: "cloudflare", AccountID: "acct", AccessKeyID: "ak", AccessKeySecret: "sk",
		BucketName: "assets", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	require.Equal(t, "acct", cf.AccountID)
	stored, err := repo.GetByUser(context.Background(), 3, cf.ID)
	require.NoError(t, err)
	require.Equal(t, "https://acct.r2.cloudflarestorage.com", stored.Endpoint)
	require.True(t, stored.ForcePathStyle)

	resolved, err := svc.Check(context.Background(), 3, view.ID, UserOSSInput{})
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotEmpty(t, storages)
}

func TestUserOSSRewriteImageAndVideo(t *testing.T) {
	repo := newUserOSSMemoryRepo()
	svc := newUserOSSServiceForTest(repo, nil)
	view, err := svc.Create(context.Background(), 4, UserOSSInput{
		Provider: "qiniuyun", AccessKey: "ak", SecretKey: "sk", Bucket: "bucket",
		Region: "cn-east-1", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	spec := UserOSSRequest{UserID: 4, RepoID: view.ID, Prefix: "file/images/"}
	png := []byte("\x89PNG\r\n\x1a\nfake")
	body, _ := json.Marshal(map[string]any{"data": []map[string]string{{"b64_json": "iVBORw0KGgo="}}})
	// The payload above is not valid png base64 of our bytes; use the real encoder via a known image item url instead.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer server.Close()
	body, _ = json.Marshal(map[string]any{"data": []map[string]string{{"url": server.URL + "/a.png"}}})
	rewritten, err := svc.RewriteImageBody(context.Background(), spec, "task 1", body)
	require.NoError(t, err)
	require.Contains(t, string(rewritten), "https://cdn.example/file/images/task-1-0.png")
	require.NotContains(t, string(rewritten), "b64_json")

	videoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "video-bytes")
	}))
	defer videoSrv.Close()
	videoBody := []byte(`{"status":"done","video":{"url":"` + videoSrv.URL + `/clip.mp4"}}`)
	out, err := svc.RewriteVideoBody(context.Background(), UserOSSRequest{UserID: 4, RepoID: view.ID, Prefix: "file/images/"}, "req/1", videoBody)
	require.NoError(t, err)
	require.Contains(t, string(out), "https://cdn.example/file/images/req-1-0.mp4")
	require.NotContains(t, string(out), videoSrv.URL)
}

func TestNormalizeOSSPathAndParseID(t *testing.T) {
	prefix, err := NormalizeOSSPath(" /file/images/ ")
	require.NoError(t, err)
	require.Equal(t, "file/images/", prefix)
	_, err = NormalizeOSSPath("../secret")
	require.Error(t, err)
	_, enabled, err := ParseOSSID("  ")
	require.NoError(t, err)
	require.False(t, enabled)
	_, _, err = ParseOSSID("abc")
	require.Error(t, err)
}

func TestImageTaskUserOSSSkipsAdminUploader(t *testing.T) {
	store := &imageTaskMemoryStore{}
	admin := &fakeImageStorage{url: "https://admin.example/images/x.png"}
	tasks := NewImageTaskServiceWithUploader(store, NewImageResultUploader(admin, "images/", 0, nil), time.Hour, time.Minute)
	repo := newUserOSSMemoryRepo()
	oss := newUserOSSServiceForTest(repo, nil)
	tasks.SetUserOSS(oss)
	created, err := tasks.Create(context.Background(), ImageTaskOwner{UserID: 4, APIKeyID: 9})
	require.NoError(t, err)
	view, err := oss.Create(context.Background(), 4, UserOSSInput{
		Provider: "s3", AccessKeyID: "ak", SecretAccessKey: "sk", Bucket: "b",
		Region: "auto", Endpoint: "https://s3.example", Domain: "https://cdn.example",
	})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nfake"))
	}))
	defer server.Close()
	body, _ := json.Marshal(map[string]any{"data": []map[string]string{{"url": server.URL + "/a.png"}}})
	require.NoError(t, tasks.CompleteWithUserOSS(context.Background(), created.ID, http.StatusOK, body, UserOSSRequest{
		UserID: 4, RepoID: view.ID, Prefix: "out/",
	}))
	got, err := tasks.Get(context.Background(), ImageTaskOwner{UserID: 4, APIKeyID: 9}, created.ID)
	require.NoError(t, err)
	require.Contains(t, got.ImageURL, "https://cdn.example/out/")
	require.Empty(t, admin.saved)
}

func mustList(t *testing.T, svc *UserOSSService, userID int64) []*UserOSSView {
	t.Helper()
	items, err := svc.List(context.Background(), userID)
	require.NoError(t, err)
	return items
}
