package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	UserOSSProviderAliyun     = "aliyun"
	UserOSSProviderTencent    = "tencent"
	UserOSSProviderQiniu      = "qiniuyun"
	UserOSSProviderCloudflare = "cloudflare"
	UserOSSProviderS3         = "s3"

	maxUserOSSRepositories = 20
)

var (
	ErrUserOSSNotFound = infraerrors.NotFound("USER_OSS_NOT_FOUND", "object storage repository not found")

	errUserOSSSecretRequired = errors.New("secret is required")
	errUserOSSDomain         = errors.New("domain must be an http(s) URL")
	errUserOSSBucketURL      = errors.New("bucket_url must look like https://{bucket}-{appid}.cos.{region}.myqcloud.com")
	errUserOSSTooMany        = errors.New("too many object storage repositories")

	tencentBucketURL    = regexp.MustCompile(`^(?:https?://)?([a-z0-9][a-z0-9-]*-[0-9]+)\.cos\.([a-z0-9-]+)\.myqcloud\.com/?$`)
	userOSSRegionPat    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)
	userOSSPathSegment  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	cloudflareAccountID = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

// UserOSSRepository is the per-user store. Queries must always include user_id.
type UserOSSRepository interface {
	ListByUser(ctx context.Context, userID int64) ([]*UserOSSRecord, error)
	GetByUser(ctx context.Context, userID, id int64) (*UserOSSRecord, error)
	Create(ctx context.Context, rec *UserOSSRecord) (*UserOSSRecord, error)
	Update(ctx context.Context, rec *UserOSSRecord) error
	Delete(ctx context.Context, userID, id int64) error
}

// UserOSSRecord is the persisted row. SecretEncrypted is ciphertext.
type UserOSSRecord struct {
	ID              int64
	UserID          int64
	Provider        string
	Bucket          string
	Domain          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretEncrypted string
	AccountID       string
	ForcePathStyle  bool
	BucketURL       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UserOSSInput is the create/update payload. Empty secrets on update keep the stored secret.
type UserOSSInput struct {
	Provider        string `json:"provider"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	Domain          string `json:"domain"`
	SecretID        string `json:"secret_id"`
	SecretKey       string `json:"secret_key"`
	BucketURL       string `json:"bucket_url"`
	AccessKey       string `json:"access_key"`
	AccountID       string `json:"account_id"`
	BucketName      string `json:"bucket_name"`
	Endpoint        string `json:"endpoint"`
	SecretAccessKey string `json:"secret_access_key"`
	ForcePathStyle  bool   `json:"force_path_style"`
}

// UserOSSView is the API representation. Secrets are never included.
type UserOSSView struct {
	ID               int64     `json:"id"`
	Provider         string    `json:"provider"`
	Bucket           string    `json:"bucket"`
	Domain           string    `json:"domain"`
	Region           string    `json:"region,omitempty"`
	Endpoint         string    `json:"endpoint,omitempty"`
	AccessKeyID      string    `json:"access_key_id,omitempty"`
	AccountID        string    `json:"account_id,omitempty"`
	BucketURL        string    `json:"bucket_url,omitempty"`
	ForcePathStyle   bool      `json:"force_path_style,omitempty"`
	SecretConfigured bool      `json:"secret_configured"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// EncryptionKeyGate reports whether durable secrets can be encrypted with a stable key.
type EncryptionKeyGate interface {
	EncryptionKeyConfigured() bool
}

// UserOSSService owns CRUD, connectivity checks, and per-request uploaders.
type UserOSSService struct {
	repo              UserOSSRepository
	encryptor         SecretEncryptor
	keys              EncryptionKeyGate
	factory           ImageStorageFactory
	allowPrivateFetch bool
}

func NewUserOSSService(repo UserOSSRepository, encryptor SecretEncryptor, keys EncryptionKeyGate, factory ImageStorageFactory) *UserOSSService {
	return &UserOSSService{repo: repo, encryptor: encryptor, keys: keys, factory: factory}
}

func (s *UserOSSService) List(ctx context.Context, userID int64) ([]*UserOSSView, error) {
	rows, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*UserOSSView, 0, len(rows))
	for _, row := range rows {
		out = append(out, presentUserOSS(row))
	}
	return out, nil
}

func (s *UserOSSService) Create(ctx context.Context, userID int64, in UserOSSInput) (*UserOSSView, error) {
	existing, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= maxUserOSSRepositories {
		return nil, invalidStorageConfig(errUserOSSTooMany)
	}
	rec, err := s.normalize(ctx, userID, 0, in, nil)
	if err != nil {
		return nil, err
	}
	created, err := s.repo.Create(ctx, rec)
	if err != nil {
		return nil, err
	}
	return presentUserOSS(created), nil
}

func (s *UserOSSService) Update(ctx context.Context, userID, id int64, in UserOSSInput) (*UserOSSView, error) {
	old, err := s.repo.GetByUser(ctx, userID, id)
	if err != nil {
		// Fail closed: a load error must not continue into a write that drops the secret.
		return nil, err
	}
	rec, err := s.normalize(ctx, userID, id, in, old)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, rec); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return presentUserOSS(updated), nil
}

func (s *UserOSSService) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, userID, id)
}

// Check probes the bucket with the saved or submitted secret. The result never includes secrets.
// An empty provider uses the stored repository as-is.
func (s *UserOSSService) Check(ctx context.Context, userID, id int64, in UserOSSInput) (*StorageResolvedEndpoint, error) {
	old, err := s.repo.GetByUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	rec := old
	if strings.TrimSpace(in.Provider) != "" {
		rec, err = s.normalize(ctx, userID, id, in, old)
		if err != nil {
			return nil, err
		}
	}
	cfg, err := s.configFromRecord(ctx, rec, "")
	if err != nil {
		return nil, err
	}
	resolved, resolveErr := resolvedStorageEndpoint(cfg.Provider, cfg.Region, cfg.Bucket, cfg.Endpoint, cfg.ForcePathStyle)
	if resolveErr != nil {
		return nil, invalidStorageConfig(resolveErr)
	}
	if s.factory == nil {
		return &resolved, errors.New("object storage client is unavailable")
	}
	storage, err := s.factory(ctx, cfg)
	if err != nil {
		return &resolved, err
	}
	head, ok := storage.(imageStorageBucketHead)
	if !ok {
		return &resolved, errors.New("object storage connection test requires HeadBucket")
	}
	if err := head.HeadBucket(ctx); err != nil {
		return &resolved, err
	}
	return &resolved, nil
}

// SpecFromHeaders resolves oss-id / oss-path for the owning user.
// An empty oss-id means the caller is not using a custom store.
func (s *UserOSSService) SpecFromHeaders(ctx context.Context, userID int64, ossID, ossPath string) (*UserOSSRequest, error) {
	id, enabled, err := ParseOSSID(ossID)
	if err != nil || !enabled {
		return nil, err
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("USER_OSS_UNAVAILABLE", "object storage is unavailable")
	}
	prefix, err := NormalizeOSSPath(ossPath)
	if err != nil {
		return nil, invalidStorageConfig(err)
	}
	if _, err := s.repo.GetByUser(ctx, userID, id); err != nil {
		return nil, err
	}
	return &UserOSSRequest{UserID: userID, RepoID: id, Prefix: prefix}, nil
}

func (s *UserOSSService) normalize(ctx context.Context, userID, id int64, in UserOSSInput, old *UserOSSRecord) (*UserOSSRecord, error) {
	provider, err := canonicalUserOSSProvider(in.Provider)
	if err != nil {
		return nil, invalidStorageConfig(err)
	}
	in = trimUserOSSInput(in)
	in.Provider = provider

	rec := &UserOSSRecord{
		ID:       id,
		UserID:   userID,
		Provider: provider,
		Domain:   normalizeOSSDomain(in.Domain),
	}
	if rec.Domain != "" {
		if err := validateOSSDomain(rec.Domain); err != nil {
			return nil, invalidStorageConfig(err)
		}
	}

	switch provider {
	case UserOSSProviderAliyun:
		if in.Region == "" || in.Bucket == "" || in.AccessKeyID == "" {
			return nil, invalidStorageConfig(errors.New("region, bucket, and access_key_id are required"))
		}
		rec.Region = in.Region
		rec.Bucket = in.Bucket
		rec.AccessKeyID = in.AccessKeyID
		rec.Endpoint = strings.TrimSpace(in.Endpoint)
	case UserOSSProviderTencent:
		bucket, region, err := parseTencentBucketURL(in.BucketURL)
		if err != nil {
			return nil, invalidStorageConfig(err)
		}
		if in.SecretID == "" {
			return nil, invalidStorageConfig(errors.New("secret_id is required"))
		}
		rec.Bucket = bucket
		rec.Region = region
		rec.BucketURL = strings.TrimSpace(in.BucketURL)
		rec.AccessKeyID = in.SecretID
	case UserOSSProviderQiniu:
		if in.Region == "" || in.Bucket == "" || in.AccessKey == "" {
			return nil, invalidStorageConfig(errors.New("region, bucket, and access_key are required"))
		}
		rec.Region = in.Region
		rec.Bucket = in.Bucket
		rec.AccessKeyID = in.AccessKey
	case UserOSSProviderCloudflare:
		accountID := strings.ToLower(in.AccountID)
		if !cloudflareAccountID.MatchString(accountID) || in.BucketName == "" || in.AccessKeyID == "" {
			return nil, invalidStorageConfig(errors.New("account_id must be 32 hex characters, and bucket_name and access_key_id are required"))
		}
		rec.AccountID = accountID
		rec.Bucket = in.BucketName
		rec.AccessKeyID = in.AccessKeyID
		rec.Region = "auto"
		rec.Endpoint = "https://" + accountID + ".r2.cloudflarestorage.com"
		rec.ForcePathStyle = true
	case UserOSSProviderS3:
		if in.Bucket == "" || in.AccessKeyID == "" {
			return nil, invalidStorageConfig(errors.New("bucket and access_key_id are required"))
		}
		rec.Bucket = in.Bucket
		rec.AccessKeyID = in.AccessKeyID
		rec.Region = in.Region
		if rec.Region == "" {
			rec.Region = "auto"
		}
		rec.Endpoint = in.Endpoint
		rec.ForcePathStyle = in.ForcePathStyle
	}

	if err := validateUserOSSBucket(rec.Bucket); err != nil {
		return nil, invalidStorageConfig(err)
	}
	rec.Region = strings.ToLower(rec.Region)
	if err := validateUserOSSRegion(rec.Region); err != nil {
		return nil, invalidStorageConfig(err)
	}
	if err := validateUserOSSEndpoint(rec.Endpoint); err != nil {
		return nil, invalidStorageConfig(err)
	}

	secret := secretFromUserOSSInput(provider, in)
	if secret == "" {
		if old == nil {
			return nil, invalidStorageConfig(errUserOSSSecretRequired)
		}
		// Empty secret keeps the stored ciphertext. Decrypt first so the value is
		// re-encrypted below; a decrypt failure aborts the update.
		plain, err := s.decryptSecret(old.SecretEncrypted)
		if err != nil {
			return nil, err
		}
		secret = plain
	}
	encrypted, err := s.encryptSecret(secret)
	if err != nil {
		return nil, err
	}
	rec.SecretEncrypted = encrypted

	cfg, err := s.configFromRecord(ctx, rec, "")
	if err != nil {
		return nil, err
	}
	resolved, err := resolvedStorageEndpoint(cfg.Provider, cfg.Region, cfg.Bucket, cfg.Endpoint, cfg.ForcePathStyle)
	if err != nil {
		return nil, invalidStorageConfig(err)
	}
	if err := validateUserOSSEndpoint(resolved.Endpoint); err != nil {
		return nil, invalidStorageConfig(err)
	}
	return rec, nil
}

func (s *UserOSSService) configFromRecord(ctx context.Context, rec *UserOSSRecord, prefix string) (*config.ImageStorageConfig, error) {
	if rec == nil {
		return nil, ErrUserOSSNotFound
	}
	secret, err := s.decryptSecret(rec.SecretEncrypted)
	if err != nil {
		return nil, err
	}
	provider := StorageProviderS3
	switch rec.Provider {
	case UserOSSProviderAliyun:
		provider = StorageProviderAliyunOSS
	case UserOSSProviderTencent:
		provider = StorageProviderTencentCOS
	case UserOSSProviderQiniu:
		provider = StorageProviderQiniu
	case UserOSSProviderCloudflare, UserOSSProviderS3:
		provider = StorageProviderS3
	}
	cfg := &config.ImageStorageConfig{
		Enabled:              true,
		Provider:             provider,
		Endpoint:             rec.Endpoint,
		Region:               rec.Region,
		Bucket:               rec.Bucket,
		AccessKeyID:          rec.AccessKeyID,
		SecretAccessKey:      secret,
		Prefix:               prefix,
		ForcePathStyle:       rec.ForcePathStyle,
		PublicBaseURL:        rec.Domain,
		PresignExpiry:        24,
		MaxDownloadByte:      defaultImageMaxDownloadBytes,
		RejectPrivateNetwork: true,
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = StorageProviderS3
	}
	_ = ctx
	return cfg, nil
}

func (s *UserOSSService) encryptSecret(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", invalidStorageConfig(errUserOSSSecretRequired)
	}
	if s.keys == nil || !s.keys.EncryptionKeyConfigured() {
		return "", ErrSecretEncryptionKeyNotConfigured
	}
	if s.encryptor == nil {
		return "", errors.New("secret encryptor is unavailable")
	}
	encrypted, err := s.encryptor.Encrypt(plain)
	if err != nil {
		return "", fmt.Errorf("encrypt oss secret: %w", err)
	}
	return encrypted, nil
}

func (s *UserOSSService) decryptSecret(encrypted string) (string, error) {
	encrypted = strings.TrimSpace(encrypted)
	if encrypted == "" {
		return "", invalidStorageConfig(errUserOSSSecretRequired)
	}
	if s.encryptor == nil {
		return "", errors.New("secret encryptor is unavailable")
	}
	plain, err := s.encryptor.Decrypt(encrypted)
	if err != nil {
		// Do not treat ciphertext as a usable secret.
		return "", fmt.Errorf("decrypt oss secret: %w", err)
	}
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", invalidStorageConfig(errUserOSSSecretRequired)
	}
	return plain, nil
}

func presentUserOSS(rec *UserOSSRecord) *UserOSSView {
	if rec == nil {
		return &UserOSSView{}
	}
	return &UserOSSView{
		ID:               rec.ID,
		Provider:         rec.Provider,
		Bucket:           rec.Bucket,
		Domain:           rec.Domain,
		Region:           rec.Region,
		Endpoint:         rec.Endpoint,
		AccessKeyID:      rec.AccessKeyID,
		AccountID:        rec.AccountID,
		BucketURL:        rec.BucketURL,
		ForcePathStyle:   rec.ForcePathStyle,
		SecretConfigured: strings.TrimSpace(rec.SecretEncrypted) != "",
		CreatedAt:        rec.CreatedAt,
		UpdatedAt:        rec.UpdatedAt,
	}
}

func canonicalUserOSSProvider(provider string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case UserOSSProviderAliyun, UserOSSProviderTencent, UserOSSProviderQiniu, UserOSSProviderCloudflare, UserOSSProviderS3:
		return strings.TrimSpace(strings.ToLower(provider)), nil
	default:
		return "", errUnknownStorageProvider
	}
}

func trimUserOSSInput(in UserOSSInput) UserOSSInput {
	in.Provider = strings.TrimSpace(in.Provider)
	in.AccessKeyID = strings.TrimSpace(in.AccessKeyID)
	in.AccessKeySecret = strings.TrimSpace(in.AccessKeySecret)
	in.Bucket = strings.TrimSpace(in.Bucket)
	in.Region = strings.TrimSpace(in.Region)
	in.Domain = strings.TrimSpace(in.Domain)
	in.SecretID = strings.TrimSpace(in.SecretID)
	in.SecretKey = strings.TrimSpace(in.SecretKey)
	in.BucketURL = strings.TrimSpace(in.BucketURL)
	in.AccessKey = strings.TrimSpace(in.AccessKey)
	in.AccountID = strings.TrimSpace(in.AccountID)
	in.BucketName = strings.TrimSpace(in.BucketName)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.SecretAccessKey = strings.TrimSpace(in.SecretAccessKey)
	return in
}

func secretFromUserOSSInput(provider string, in UserOSSInput) string {
	switch provider {
	case UserOSSProviderAliyun, UserOSSProviderCloudflare:
		return strings.TrimSpace(in.AccessKeySecret)
	case UserOSSProviderTencent, UserOSSProviderQiniu:
		return strings.TrimSpace(in.SecretKey)
	case UserOSSProviderS3:
		return strings.TrimSpace(in.SecretAccessKey)
	default:
		return ""
	}
}

func normalizeOSSDomain(domain string) string {
	return strings.TrimRight(strings.TrimSpace(domain), "/")
}

func validateOSSDomain(domain string) error {
	if strings.ContainsAny(domain, "\\ \t\r\n") {
		return errUserOSSDomain
	}
	parsed, err := url.Parse(domain)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errUserOSSDomain
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errUserOSSDomain
	}
	if urlvalidatorBlockedHost(parsed.Hostname()) {
		return errUserOSSDomain
	}
	return nil
}

func parseTencentBucketURL(raw string) (bucket, region string, err error) {
	raw = strings.TrimSpace(raw)
	matches := tencentBucketURL.FindStringSubmatch(raw)
	if matches == nil {
		return "", "", errUserOSSBucketURL
	}
	return matches[1], matches[2], nil
}

// ParseOSSID reports whether the oss-id header is present.
func ParseOSSID(raw string) (int64, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, true, invalidStorageConfig(errors.New("oss-id must be a repository id"))
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, true, invalidStorageConfig(errors.New("oss-id must be a repository id"))
	}
	return id, true, nil
}

// NormalizeOSSPath turns an optional object prefix into a key prefix.
// Empty means the repository root.
func NormalizeOSSPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	if path == "" {
		return "", nil
	}
	if strings.Contains(path, "..") || strings.ContainsAny(path, "\\ \t\r\n?#%") {
		return "", errors.New("oss-path must be a relative object prefix")
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." || !userOSSPathSegment.MatchString(part) {
			return "", errors.New("oss-path must be a relative object prefix")
		}
	}
	return path + "/", nil
}

func validateUserOSSBucket(bucket string) error {
	if bucket == "" || len(bucket) > 256 || strings.Contains(bucket, "..") || strings.ContainsAny(bucket, "/\\?#@ \t\r\n") {
		return errors.New("bucket name is invalid")
	}
	return nil
}

func validateUserOSSRegion(region string) error {
	if region == "" {
		return nil
	}
	if !userOSSRegionPat.MatchString(strings.ToLower(region)) {
		return errors.New("region contains unsupported characters")
	}
	return nil
}
