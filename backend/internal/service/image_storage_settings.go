package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const settingKeyImageStorageConfig = "image_storage_config"

// ErrImageStorageIncomplete 表示开关已打开但凭证不全，无法启用异步生图。
var ErrImageStorageIncomplete = errors.New("image storage is enabled but bucket/access_key_id/secret_access_key are incomplete")

// ImageStorageFactory 由 repository 层提供，把配置变成一个可用的对象存储实现。
// 与 BackupObjectStoreFactory 同样的注入方式，避免 service 反向依赖 repository。
type ImageStorageFactory func(ctx context.Context, cfg *config.ImageStorageConfig) (ImageStorage, error)

// ImageStorageSettings 是后台可编辑的异步生图对象存储配置。
//
// ReuseBackupS3 为真时不保存自己的厂商、端点与密钥，直接借用数据库备份已配置的对象存储，
// 只用自己的 Bucket/Prefix 区分对象；这样"数据走 backups/、图片走 images/"无需重复配置。
// Resolved 只出现在接口响应里，禁止写入 settings JSON。
type ImageStorageSettings struct {
	Enabled       bool `json:"enabled"`
	ReuseBackupS3 bool `json:"reuse_backup_s3"`

	Bucket           string `json:"bucket"` // 留空且复用备份时，沿用备份桶
	Prefix           string `json:"prefix"`
	PublicBaseURL    string `json:"public_base_url"`
	PresignExpiry    int    `json:"presign_expiry_hours"`
	MaxDownloadBytes int64  `json:"max_download_bytes"`

	// 以下仅在 ReuseBackupS3 为假时落库。复用时响应里的 provider 与 resolved 来自备份配置。
	Provider        string                   `json:"provider,omitempty"` // 空值表示 s3
	Endpoint        string                   `json:"endpoint"`
	Region          string                   `json:"region"`
	AccessKeyID     string                   `json:"access_key_id"`
	SecretAccessKey string                   `json:"secret_access_key,omitempty"` //nolint:revive // field name follows AWS convention
	ForcePathStyle  bool                     `json:"force_path_style"`
	Resolved        *StorageResolvedEndpoint `json:"resolved,omitempty"`
}

// imageStorageBucketHead is the connection probe on the concrete S3 image store.
// ImageStorage itself stays Save-only.
type imageStorageBucketHead interface {
	HeadBucket(ctx context.Context) error
}

// ImageStorageSettingService 读写后台设置，并把结果解析成一个可直接使用的 uploader。
//
// 解析结果带缓存：网关每次请求都要判断功能是否开启，不能每次都查库。保存设置时调用
// Invalidate 清缓存，下一次请求即重建客户端——这是"后台开关立即生效、无需重启"的实现。
type ImageStorageSettingService struct {
	settingRepo SettingRepository
	encryptor   SecretEncryptor
	backup      *BackupService
	factory     ImageStorageFactory

	// fallback 是 config.yaml 里的配置。后台从未保存过设置时沿用它，
	// 保证升级前已用配置文件开启该功能的部署不被打断。
	fallback config.ImageStorageConfig

	mu       sync.Mutex
	resolved bool
	uploader *ImageResultUploader
	enabled  bool
}

func NewImageStorageSettingService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	backup *BackupService,
	factory ImageStorageFactory,
	fallback config.ImageStorageConfig,
) *ImageStorageSettingService {
	return &ImageStorageSettingService{
		settingRepo: settingRepo,
		encryptor:   encryptor,
		backup:      backup,
		factory:     factory,
		fallback:    fallback,
	}
}

// Resolver 返回可注入 ImageTaskService 的解析函数。
func (s *ImageStorageSettingService) Resolver() ImageStorageResolver {
	return func() (*ImageResultUploader, bool) {
		return s.resolve()
	}
}

func (s *ImageStorageSettingService) resolve() (*ImageResultUploader, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolved {
		return s.uploader, s.enabled
	}

	ctx := context.Background()
	s.resolved = true
	s.uploader, s.enabled = nil, false

	cfg, err := s.effectiveConfig(ctx)
	if err != nil {
		logger.L().Warn("image_storage.settings_load_failed; async image tasks stay disabled", zap.Error(err))
		return nil, false
	}
	if !cfg.Enabled {
		return nil, false
	}
	if !cfg.IsConfigured() {
		logger.L().Warn("image_storage is enabled but not fully configured; async image tasks are disabled",
			zap.Strings("missing_keys", cfg.MissingCredentialKeys()))
		return nil, false
	}

	storage, err := s.factory(ctx, cfg)
	if err != nil {
		logger.L().Error("image_storage.client_build_failed; async image tasks stay disabled", zap.Error(err))
		return nil, false
	}
	s.uploader = NewImageResultUploader(storage, cfg.Prefix, cfg.MaxDownloadByte, nil)
	s.enabled = true
	return s.uploader, true
}

// Invalidate 丢弃缓存，使下一次请求按最新设置重新解析。
func (s *ImageStorageSettingService) Invalidate() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.resolved = false
	s.uploader = nil
	s.enabled = false
	s.mu.Unlock()
}

// Get 返回后台设置（SecretAccessKey 已脱敏）。从未保存过时返回 config.yaml 的等价值。
func (s *ImageStorageSettingService) Get(ctx context.Context) (*ImageStorageSettings, error) {
	settings, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		fallback := s.fallback
		if err := prepareFileImageStorage(&fallback); err != nil {
			return nil, err
		}
		settings = settingsFromConfig(fallback)
	}
	return s.present(ctx, settings)
}

// SecretConfigured 供前端展示"已配置"占位符。
// 复用备份时反映备份密钥；取消复用后应改看 OwnSecretConfigured。
func (s *ImageStorageSettingService) SecretConfigured(ctx context.Context) bool {
	settings, err := s.load(ctx)
	if err != nil {
		return false
	}
	if settings == nil {
		return strings.TrimSpace(s.fallback.SecretAccessKey) != ""
	}
	if settings.ReuseBackupS3 {
		cfg, err := s.backupCredentials(ctx)
		return err == nil && cfg != nil && strings.TrimSpace(cfg.SecretAccessKey) != ""
	}
	return strings.TrimSpace(settings.SecretAccessKey) != ""
}

// OwnSecretConfigured reports whether the image-storage row itself has a secret.
// Reuse mode stores no image secret, so this stays false even when the backup secret exists.
func (s *ImageStorageSettingService) OwnSecretConfigured(ctx context.Context) bool {
	settings, err := s.load(ctx)
	if err != nil {
		return false
	}
	if settings == nil {
		return strings.TrimSpace(s.fallback.SecretAccessKey) != ""
	}
	return strings.TrimSpace(settings.SecretAccessKey) != ""
}

// Update 保存设置并立即生效。SecretAccessKey 留空表示沿用已保存的值。
func (s *ImageStorageSettingService) Update(ctx context.Context, in ImageStorageSettings) (*ImageStorageSettings, error) {
	normalizeImageStorageSettings(&in)
	in.Resolved = nil

	if in.ReuseBackupS3 {
		// 复用备份凭证时不落自己的厂商和密钥，避免同一份密钥在库里存两份。
		in.Provider = ""
		in.Endpoint, in.Region, in.AccessKeyID, in.SecretAccessKey = "", "", "", ""
		in.ForcePathStyle = false
	} else {
		provider, err := canonicalStorageProvider(in.Provider)
		if err != nil {
			return nil, invalidStorageConfig(err)
		}
		in.Provider = provider
		if in.SecretAccessKey == "" {
			old, err := s.load(ctx)
			if err != nil {
				return nil, err
			}
			if old != nil {
				in.SecretAccessKey = old.SecretAccessKey
			}
		} else {
			// 拒绝用自动生成的临时密钥加密：重启后密文无法解密（#4524）。
			// 与备份 S3 配置共用同一把密钥，故复用其配置状态判断。
			if s.backup == nil || !s.backup.EncryptionKeyConfigured() {
				return nil, ErrSecretEncryptionKeyNotConfigured
			}
			encrypted, err := s.encryptor.Encrypt(in.SecretAccessKey)
			if err != nil {
				return nil, fmt.Errorf("encrypt secret: %w", err)
			}
			in.SecretAccessKey = encrypted
		}
	}

	presented, err := s.present(ctx, &in)
	if err != nil {
		if isStorageConfigValidation(err) {
			return nil, invalidStorageConfig(err)
		}
		return nil, err
	}

	in.Resolved = nil
	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal image storage settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyImageStorageConfig, string(data)); err != nil {
		return nil, fmt.Errorf("save image storage settings: %w", err)
	}
	s.Invalidate()
	return presented, nil
}

// TestConnection 用给定设置试建一次客户端，用于后台的"测试连接"按钮。
// 与 Update 一样支持留空 SecretAccessKey 表示沿用已保存的值。
// 返回的 resolved 只含端点、签名区域和 path-style，不含密钥。
func (s *ImageStorageSettingService) TestConnection(ctx context.Context, in ImageStorageSettings) (*StorageResolvedEndpoint, error) {
	normalizeImageStorageSettings(&in)
	if !in.ReuseBackupS3 && in.SecretAccessKey == "" {
		old, err := s.load(ctx)
		if err != nil {
			return nil, err
		}
		if old != nil {
			in.SecretAccessKey = old.SecretAccessKey
		}
	}
	cfg, err := s.toImageStorageConfig(ctx, &in)
	if err != nil {
		return nil, err
	}
	resolved, resolveErr := resolvedStorageEndpoint(cfg.Provider, cfg.Region, cfg.Bucket, cfg.Endpoint, cfg.ForcePathStyle)
	if !cfg.IsConfigured() {
		if resolveErr != nil {
			return nil, ErrImageStorageIncomplete
		}
		return &resolved, ErrImageStorageIncomplete
	}
	if resolveErr != nil {
		return nil, resolveErr
	}
	storage, err := s.factory(ctx, cfg)
	if err != nil {
		return &resolved, err
	}
	head, ok := storage.(imageStorageBucketHead)
	if !ok {
		return &resolved, errors.New("image storage connection test requires HeadBucket")
	}
	if err := head.HeadBucket(ctx); err != nil {
		return &resolved, err
	}
	return &resolved, nil
}

// prepareFileImageStorage normalizes the config.yaml / environment fallback.
// region "auto" is the s3 example sentinel. The admin UI clears it on provider
// change; file and env config must not derive a non-s3 endpoint from it.
func prepareFileImageStorage(cfg *config.ImageStorageConfig) error {
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	if cfg.Provider == "" {
		cfg.Provider = StorageProviderS3
	}
	cfg.Region = strings.TrimSpace(cfg.Region)
	switch cfg.Provider {
	case StorageProviderAliyunOSS, StorageProviderTencentCOS, StorageProviderQiniu:
		if cfg.Region == "auto" {
			return invalidStorageConfig(errStorageRegionRequired)
		}
	}
	return nil
}

// effectiveConfig 把后台设置（或 config.yaml 回落）解析成运行时配置。
func (s *ImageStorageSettingService) effectiveConfig(ctx context.Context) (*config.ImageStorageConfig, error) {
	settings, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		fallback := s.fallback
		if err := prepareFileImageStorage(&fallback); err != nil {
			return nil, err
		}
		return &fallback, nil
	}
	return s.toImageStorageConfig(ctx, settings)
}

func (s *ImageStorageSettingService) toImageStorageConfig(ctx context.Context, in *ImageStorageSettings) (*config.ImageStorageConfig, error) {
	cfg := &config.ImageStorageConfig{
		Enabled:         in.Enabled,
		Provider:        strings.TrimSpace(in.Provider),
		Bucket:          in.Bucket,
		Prefix:          in.Prefix,
		PublicBaseURL:   in.PublicBaseURL,
		PresignExpiry:   in.PresignExpiry,
		MaxDownloadByte: in.MaxDownloadBytes,
		Endpoint:        in.Endpoint,
		Region:          in.Region,
		AccessKeyID:     in.AccessKeyID,
		SecretAccessKey: in.SecretAccessKey,
		ForcePathStyle:  in.ForcePathStyle,
	}

	if in.ReuseBackupS3 {
		backupCfg, err := s.backupCredentials(ctx)
		if err != nil {
			return nil, err
		}
		if backupCfg == nil {
			return nil, errors.New("image storage is set to reuse the backup S3 configuration, but no backup S3 configuration exists")
		}
		cfg.Provider = strings.TrimSpace(backupCfg.Provider)
		cfg.Endpoint = backupCfg.Endpoint
		cfg.Region = backupCfg.Region
		cfg.AccessKeyID = backupCfg.AccessKeyID
		cfg.SecretAccessKey = backupCfg.SecretAccessKey
		cfg.ForcePathStyle = backupCfg.ForcePathStyle
		if cfg.Bucket == "" {
			cfg.Bucket = backupCfg.Bucket
		}
	} else if cfg.SecretAccessKey != "" {
		decrypted, err := s.encryptor.Decrypt(cfg.SecretAccessKey)
		if err != nil {
			// 兼容未加密的旧数据，与备份配置的处理保持一致。
			logger.L().Warn("image_storage secret decrypt failed; treating the stored value as plaintext", zap.Error(err))
		} else {
			cfg.SecretAccessKey = decrypted
		}
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = StorageProviderS3
	}
	return cfg, nil
}

// backupCredentials 取备份已配置的 S3 凭证（已解密）。
func (s *ImageStorageSettingService) backupCredentials(ctx context.Context) (*BackupS3Config, error) {
	if s.backup == nil {
		return nil, errors.New("backup service is unavailable")
	}
	return s.backup.loadS3Config(ctx)
}

// load 读出后台设置；从未保存过时返回 nil。
func (s *ImageStorageSettingService) load(ctx context.Context) (*ImageStorageSettings, error) {
	if s.settingRepo == nil {
		return nil, nil //nolint:nilnil // no repository means no stored settings
	}
	raw, err := s.settingRepo.GetValue(ctx, settingKeyImageStorageConfig)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("load image storage settings: %w", err)
	}
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, nil //nolint:nilnil // never configured is a valid state
	}
	var settings ImageStorageSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, fmt.Errorf("parse image storage settings: %w", err)
	}
	return &settings, nil
}

func settingsFromConfig(cfg config.ImageStorageConfig) *ImageStorageSettings {
	return &ImageStorageSettings{
		Enabled:          cfg.Enabled,
		Provider:         cfg.Provider,
		Bucket:           cfg.Bucket,
		Prefix:           cfg.Prefix,
		PublicBaseURL:    cfg.PublicBaseURL,
		PresignExpiry:    cfg.PresignExpiry,
		MaxDownloadBytes: cfg.MaxDownloadByte,
		Endpoint:         cfg.Endpoint,
		Region:           cfg.Region,
		AccessKeyID:      cfg.AccessKeyID,
		SecretAccessKey:  cfg.SecretAccessKey,
		ForcePathStyle:   cfg.ForcePathStyle,
	}
}

// present builds the admin response. Reuse keeps the stored image endpoint and keys
// wiped, and fills provider plus resolved from the backup config after resolution.
func (s *ImageStorageSettingService) present(ctx context.Context, in *ImageStorageSettings) (*ImageStorageSettings, error) {
	if in == nil {
		in = &ImageStorageSettings{}
	}
	out := *in
	out.SecretAccessKey = ""
	out.Resolved = nil

	provider := out.Provider
	region := out.Region
	bucket := out.Bucket
	endpoint := out.Endpoint
	forcePathStyle := out.ForcePathStyle
	if out.ReuseBackupS3 {
		backup, err := s.backupCredentials(ctx)
		if err != nil {
			return nil, err
		}
		if backup != nil {
			provider = backup.Provider
			region = backup.Region
			endpoint = backup.Endpoint
			forcePathStyle = backup.ForcePathStyle
			if bucket == "" {
				bucket = backup.Bucket
			}
		}
	}

	canonical, err := canonicalStorageProvider(provider)
	if err != nil {
		return nil, err
	}
	out.Provider = canonical
	resolved, err := resolvedStorageEndpoint(canonical, region, bucket, endpoint, forcePathStyle)
	if err != nil {
		return nil, err
	}
	out.Resolved = &resolved
	return &out, nil
}

func normalizeImageStorageSettings(in *ImageStorageSettings) {
	in.Bucket = strings.TrimSpace(in.Bucket)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.Region = strings.TrimSpace(in.Region)
	in.AccessKeyID = strings.TrimSpace(in.AccessKeyID)
	in.SecretAccessKey = strings.TrimSpace(in.SecretAccessKey)
	in.PublicBaseURL = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(in.PublicBaseURL), "/"))

	in.Provider = strings.TrimSpace(in.Provider)
	in.Prefix = strings.TrimSpace(in.Prefix)
	if in.Prefix == "" {
		in.Prefix = "images/"
	}
	if !strings.HasSuffix(in.Prefix, "/") {
		in.Prefix += "/"
	}
	// s3（含未填写的厂商）沿用历史默认 region。其它厂商禁止把空 region 写成 auto。
	if (in.Provider == "" || in.Provider == StorageProviderS3) && in.Region == "" {
		in.Region = "auto"
	}
	if in.PresignExpiry <= 0 {
		in.PresignExpiry = 24
	}
	if in.MaxDownloadBytes <= 0 {
		in.MaxDownloadBytes = defaultImageMaxDownloadBytes
	}
}
