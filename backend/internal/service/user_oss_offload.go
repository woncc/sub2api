package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const userOSSVideoMaxBytes int64 = 256 << 20

type userOSSRequestKey struct{}
type ossOwnerKey struct{}

// UserOSSRequest is the per-call custom store selection.
// Defer means the caller will upload later (async image completion) and the
// synchronous response writer must leave the upstream body unchanged.
type UserOSSRequest struct {
	UserID int64
	RepoID int64
	Prefix string
	Defer  bool
}

// OSSOwner identifies the API key user that may resolve an oss-id.
type OSSOwner struct {
	UserID   int64
	APIKeyID int64
}

// AttachUserOSS stores the resolved repository on the request context.
func AttachUserOSS(c *gin.Context, spec UserOSSRequest) {
	if c == nil || c.Request == nil {
		return
	}
	ctx := context.WithValue(c.Request.Context(), userOSSRequestKey{}, spec)
	c.Request = c.Request.WithContext(ctx)
}

// AttachOSSOwner stores the API key owner so later status polls can load a saved binding.
func AttachOSSOwner(c *gin.Context, userID, apiKeyID int64) {
	if c == nil || c.Request == nil || userID <= 0 {
		return
	}
	ctx := context.WithValue(c.Request.Context(), ossOwnerKey{}, OSSOwner{UserID: userID, APIKeyID: apiKeyID})
	c.Request = c.Request.WithContext(ctx)
}

func UserOSSRequestFromContext(ctx context.Context) (UserOSSRequest, bool) {
	if ctx == nil {
		return UserOSSRequest{}, false
	}
	spec, ok := ctx.Value(userOSSRequestKey{}).(UserOSSRequest)
	return spec, ok && spec.RepoID > 0 && spec.UserID > 0
}

func OSSOwnerFromContext(ctx context.Context) (OSSOwner, bool) {
	if ctx == nil {
		return OSSOwner{}, false
	}
	owner, ok := ctx.Value(ossOwnerKey{}).(OSSOwner)
	return owner, ok && owner.UserID > 0
}

// RewriteImageBody uploads image artifacts and returns URLs on the user's domain.
func (s *UserOSSService) RewriteImageBody(ctx context.Context, spec UserOSSRequest, taskID string, body []byte) ([]byte, error) {
	cfg, uploader, err := s.imageUploader(ctx, spec)
	if err != nil {
		return nil, err
	}
	rewritten, err := uploader.Rewrite(ctx, sanitizeOSSKeyPart(taskID), body)
	if err != nil {
		return nil, fmt.Errorf("upload image to user object storage: %w", err)
	}
	if err := assertRewrittenImageURLs(rewritten, cfg.PublicBaseURL); err != nil {
		return nil, fmt.Errorf("upload image to user object storage: %w", err)
	}
	return rewritten, nil
}

// RewriteVideoBody uploads completed video URLs. A body without an http(s) video URL is unchanged.
func (s *UserOSSService) RewriteVideoBody(ctx context.Context, spec UserOSSRequest, taskID string, body []byte) ([]byte, error) {
	if !videoArtifactPresent(body) {
		return body, nil
	}
	cfg, storage, err := s.openStorage(ctx, spec)
	if err != nil {
		return nil, err
	}
	client := s.fetchClient()
	uploaded := map[string]string{}
	index := 0
	for _, path := range []string{"video.url", "content.video_url", "video_url"} {
		raw := strings.TrimSpace(gjson.GetBytes(body, path).String())
		if !isHTTPURL(raw) {
			continue
		}
		if !s.allowPrivateFetch {
			if err := validateUserOSSFetchURL(raw); err != nil {
				return nil, err
			}
		}
		target, ok := uploaded[raw]
		if !ok {
			data, contentType, err := downloadOSSObject(ctx, client, raw, userOSSVideoMaxBytes)
			if err != nil {
				return nil, fmt.Errorf("download video: %w", err)
			}
			key := spec.Prefix + sanitizeOSSKeyPart(taskID) + "-" + fmt.Sprint(index) + extensionForVideoContentType(contentType)
			index++
			target, err = storage.Save(ctx, key, contentType, data)
			if err != nil {
				return nil, fmt.Errorf("upload video to user object storage: %w", err)
			}
			if err := artifactURLAllowed(cfg.PublicBaseURL, target); err != nil {
				return nil, err
			}
			uploaded[raw] = target
		}
		body, err = sjson.SetBytes(body, path, target)
		if err != nil {
			return nil, err
		}
	}
	if len(uploaded) == 0 {
		return nil, errors.New("video response did not include an uploadable url")
	}
	return body, nil
}

func (s *UserOSSService) imageUploader(ctx context.Context, spec UserOSSRequest) (*config.ImageStorageConfig, *ImageResultUploader, error) {
	cfg, storage, err := s.openStorage(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	return cfg, NewImageResultUploader(storage, cfg.Prefix, cfg.MaxDownloadByte, s.fetchClient()), nil
}

func (s *UserOSSService) fetchClient() *http.Client {
	if s != nil && s.allowPrivateFetch {
		return &http.Client{Timeout: 30 * time.Second}
	}
	return NewUserOSSHTTPClient()
}

func (s *UserOSSService) openStorage(ctx context.Context, spec UserOSSRequest) (*config.ImageStorageConfig, ImageStorage, error) {
	if s == nil || s.repo == nil || s.factory == nil {
		return nil, nil, infraerrors.ServiceUnavailable("USER_OSS_UNAVAILABLE", "object storage is unavailable")
	}
	rec, err := s.repo.GetByUser(ctx, spec.UserID, spec.RepoID)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := s.configFromRecord(ctx, rec, spec.Prefix)
	if err != nil {
		return nil, nil, err
	}
	if !cfg.IsConfigured() {
		return nil, nil, invalidStorageConfig(errors.New("object storage repository is incomplete"))
	}
	storage, err := s.factory(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("build user object storage client: %w", err)
	}
	return cfg, storage, nil
}

func assertRewrittenImageURLs(body []byte, publicBase string) error {
	urls := gjson.GetBytes(body, "data.#.url")
	if !urls.IsArray() || len(urls.Array()) == 0 {
		return errors.New("image response did not include an uploadable image")
	}
	for _, item := range urls.Array() {
		if err := artifactURLAllowed(publicBase, item.String()); err != nil {
			return err
		}
	}
	return nil
}

func videoArtifactPresent(body []byte) bool {
	for _, path := range []string{"video.url", "content.video_url", "video_url"} {
		if isHTTPURL(gjson.GetBytes(body, path).String()) {
			return true
		}
	}
	return false
}

func isHTTPURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://")
}

func downloadOSSObject(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, string, error) {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if limit <= 0 {
		limit = userOSSVideoMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("object exceeds %d bytes", limit)
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func extensionForVideoContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "webm"):
		return ".webm"
	case strings.Contains(ct, "quicktime"):
		return ".mov"
	case strings.Contains(ct, "mp4"), strings.Contains(ct, "video"):
		return ".mp4"
	default:
		return ".mp4"
	}
}

func sanitizeOSSKeyPart(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "artifact"
	}
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "artifact"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

// rewriteUserOSSImage uploads a synchronous image response when the request selected a user store.
func (s *OpenAIGatewayService) rewriteUserOSSImage(c *gin.Context, taskID string, body []byte) ([]byte, error) {
	if s == nil || s.userOSS == nil || c == nil || c.Request == nil {
		return body, nil
	}
	spec, ok := UserOSSRequestFromContext(c.Request.Context())
	if !ok || spec.Defer {
		return body, nil
	}
	return s.userOSS.RewriteImageBody(c.Request.Context(), spec, taskID, body)
}

// rewriteUserOSSVideo uploads a completed video when the request or the create-time binding selected a user store.
// applied is true when the body was rewritten, so callers can skip the default content-proxy URL.
func (s *OpenAIGatewayService) rewriteUserOSSVideo(c *gin.Context, requestID string, body []byte) ([]byte, bool, error) {
	if s == nil || s.userOSS == nil || c == nil || c.Request == nil || !videoArtifactPresent(body) {
		return body, false, nil
	}
	spec, ok := UserOSSRequestFromContext(c.Request.Context())
	if !ok || spec.Defer {
		owner, ownerOK := OSSOwnerFromContext(c.Request.Context())
		if !ownerOK || strings.TrimSpace(requestID) == "" {
			return body, false, nil
		}
		pending, err := s.LoadGrokVideoPendingBilling(c.Request.Context(), requestID, owner.UserID, owner.APIKeyID)
		if err != nil {
			return nil, false, fmt.Errorf("load user oss binding: %w", err)
		}
		if pending == nil || pending.OSSRepositoryID <= 0 {
			return body, false, nil
		}
		spec = UserOSSRequest{UserID: owner.UserID, RepoID: pending.OSSRepositoryID, Prefix: pending.OSSPath}
	}
	rewritten, err := s.userOSS.RewriteVideoBody(c.Request.Context(), spec, requestID, body)
	if err != nil {
		return nil, false, err
	}
	if bytes.Equal(rewritten, body) {
		return body, false, nil
	}
	return rewritten, true, nil
}

// SetUserOSS installs the per-user store used when generation requests carry oss-id.
func (s *OpenAIGatewayService) SetUserOSS(oss *UserOSSService) {
	if s != nil {
		s.userOSS = oss
	}
}

// persistUserOSSBinding stores oss-id before the create response is written.
// A later status poll must still upload when the client does not repeat the header.
// Store failure aborts the success response instead of silently using platform storage.
func (s *OpenAIGatewayService) persistUserOSSBinding(c *gin.Context, requestID string) error {
	if s == nil || c == nil || c.Request == nil {
		return nil
	}
	spec, ok := UserOSSRequestFromContext(c.Request.Context())
	if !ok || spec.RepoID <= 0 || spec.Defer {
		return nil
	}
	owner, ok := OSSOwnerFromContext(c.Request.Context())
	if !ok {
		return fmt.Errorf("user object storage binding is missing an owner")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("user object storage binding is missing a task id")
	}
	pending, err := s.LoadGrokVideoPendingBilling(c.Request.Context(), requestID, owner.UserID, owner.APIKeyID)
	if err != nil {
		return fmt.Errorf("load user oss binding: %w", err)
	}
	if pending == nil {
		pending = &GrokVideoPendingBilling{CreatedAt: GrokVideoPendingCreatedAtNow()}
	}
	pending.OSSRepositoryID = spec.RepoID
	pending.OSSPath = spec.Prefix
	if err := s.StoreGrokVideoPendingBilling(c.Request.Context(), requestID, owner.UserID, owner.APIKeyID, *pending); err != nil {
		return fmt.Errorf("store user oss binding: %w", err)
	}
	return nil
}

func isUserOSSVideoCreate(endpoint GrokMediaEndpoint) bool {
	switch endpoint {
	case GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}
