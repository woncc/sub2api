//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type stubSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
	getErr error
}

func newStubSettingRepo() *stubSettingRepo {
	return &stubSettingRepo{values: map[string]string{}}
}

func (r *stubSettingRepo) Get(context.Context, string) (*Setting, error) { return nil, nil }
func (r *stubSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return "", r.getErr
	}
	return r.values[key], nil
}

func (r *stubSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}
func (r *stubSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *stubSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (r *stubSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *stubSettingRepo) Delete(context.Context, string) error { return nil }

// reversibleEncryptor stands in for AES: prefixed so a test can tell ciphertext
// from plaintext, and so decrypting a plaintext value fails like the real one.
type reversibleEncryptor struct{}

func (reversibleEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (reversibleEncryptor) Decrypt(ciphertext string) (string, error) {
	rest, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", errors.New("not encrypted")
	}
	return rest, nil
}

type recordingStorage struct{ saved []string }

func (s *recordingStorage) Save(_ context.Context, key, _ string, _ []byte) (string, error) {
	s.saved = append(s.saved, key)
	return "https://cdn.example.com/" + key, nil
}

func newImageStorageFixture(t *testing.T, fallback config.ImageStorageConfig) (*ImageStorageSettingService, *stubSettingRepo, *[]config.ImageStorageConfig) {
	return newImageStorageFixtureWithKey(t, fallback, true)
}

func newImageStorageFixtureWithKey(t *testing.T, fallback config.ImageStorageConfig, encryptionKeyConfigured bool) (*ImageStorageSettingService, *stubSettingRepo, *[]config.ImageStorageConfig) {
	t.Helper()
	repo := newStubSettingRepo()
	encryptor := reversibleEncryptor{}
	backup := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: encryptionKeyConfigured},
	}, encryptor, nil, nil)

	var built []config.ImageStorageConfig
	factory := func(_ context.Context, cfg *config.ImageStorageConfig) (ImageStorage, error) {
		built = append(built, *cfg)
		return &recordingStorage{}, nil
	}
	return NewImageStorageSettingService(repo, encryptor, backup, factory, fallback), repo, &built
}

func seedBackupS3(t *testing.T, repo *stubSettingRepo, cfg BackupS3Config) {
	t.Helper()
	cfg.SecretAccessKey = "enc:" + cfg.SecretAccessKey
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), settingKeyBackupS3Config, string(data)))
}

// The admin switch must take effect without a restart: that is the entire point
// of moving image_storage out of config.yaml (#4542).
func TestImageStorageSettingsToggleTakesEffectWithoutRestart(t *testing.T) {
	svc, repo, built := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()
	seedBackupS3(t, repo, BackupS3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "backup-bucket", AccessKeyID: "ak", SecretAccessKey: "sk",
		Prefix: "backups/",
	})

	uploader, enabled := svc.resolve()
	require.False(t, enabled, "disabled until an admin turns it on")
	require.Nil(t, uploader)

	_, err := svc.Update(ctx, ImageStorageSettings{Enabled: true, ReuseBackupS3: true})
	require.NoError(t, err)

	uploader, enabled = svc.resolve()
	require.True(t, enabled, "saving the setting must enable the feature immediately")
	require.NotNil(t, uploader)

	_, err = svc.Update(ctx, ImageStorageSettings{Enabled: false, ReuseBackupS3: true})
	require.NoError(t, err)
	_, enabled = svc.resolve()
	require.False(t, enabled, "turning it back off must also apply immediately")

	require.Len(t, *built, 1, "the S3 client is built only when the feature is on")
}

func TestImageStorageSettingsReuseBackupCredentials(t *testing.T) {
	svc, repo, built := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()
	seedBackupS3(t, repo, BackupS3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "wnam",
		Bucket: "backup-bucket", AccessKeyID: "backup-ak", SecretAccessKey: "backup-sk",
		Prefix: "backups/", ForcePathStyle: true,
	})

	_, err := svc.Update(ctx, ImageStorageSettings{Enabled: true, ReuseBackupS3: true, Prefix: "images"})
	require.NoError(t, err)
	_, enabled := svc.resolve()
	require.True(t, enabled)

	require.Len(t, *built, 1)
	got := (*built)[0]
	require.Equal(t, "https://acct.r2.cloudflarestorage.com", got.Endpoint)
	require.Equal(t, "wnam", got.Region)
	require.Equal(t, "backup-ak", got.AccessKeyID)
	require.Equal(t, "backup-sk", got.SecretAccessKey, "the backup secret must be decrypted before use")
	require.True(t, got.ForcePathStyle)
	require.Equal(t, "backup-bucket", got.Bucket, "an empty bucket falls back to the backup bucket")
	require.Equal(t, "images/", got.Prefix, "images stay under their own prefix so they never collide with backups/")

	// Reusing must not duplicate the secret into a second row.
	raw, err := repo.GetValue(ctx, settingKeyImageStorageConfig)
	require.NoError(t, err)
	require.NotContains(t, raw, "backup-sk")
	require.NotContains(t, raw, "enc:")
}

func TestImageStorageSettingsOwnCredentialsAreEncryptedAndMasked(t *testing.T) {
	svc, repo, built := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()

	saved, err := svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Bucket: "my-images",
		Endpoint:    "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "ak", SecretAccessKey: "super-secret",
	})
	require.NoError(t, err)
	require.Empty(t, saved.SecretAccessKey, "the response must never echo the secret back")

	raw, err := repo.GetValue(ctx, settingKeyImageStorageConfig)
	require.NoError(t, err)
	require.NotContains(t, raw, `"secret_access_key":"super-secret"`, "the secret must be encrypted at rest")
	require.Contains(t, raw, "enc:super-secret")

	fetched, err := svc.Get(ctx)
	require.NoError(t, err)
	require.Empty(t, fetched.SecretAccessKey)
	require.True(t, svc.SecretConfigured(ctx))

	_, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, "super-secret", (*built)[0].SecretAccessKey, "the stored secret must be decrypted before use")

	// An update that omits the secret keeps the stored one rather than wiping it.
	_, err = svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Bucket: "my-images",
		Endpoint: "https://acct.r2.cloudflarestorage.com", AccessKeyID: "ak",
	})
	require.NoError(t, err)
	svc.resolve()
	require.Equal(t, "super-secret", (*built)[1].SecretAccessKey)
}

// Persisting the service's own S3 secret must be refused when the encryption key
// is auto-generated, otherwise the ciphertext cannot be decrypted after a
// restart (#4524). Reusing the backup credentials stays allowed because it does
// not persist a second copy of the secret.
func TestImageStorageSettingsRejectSecretWithEphemeralKey(t *testing.T) {
	svc, repo, built := newImageStorageFixtureWithKey(t, config.ImageStorageConfig{}, false)
	ctx := context.Background()

	_, err := svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Bucket: "my-images",
		Endpoint:    "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "ak", SecretAccessKey: "super-secret",
	})
	require.ErrorIs(t, err, ErrSecretEncryptionKeyNotConfigured)

	raw, _ := repo.GetValue(ctx, settingKeyImageStorageConfig)
	require.Empty(t, raw, "nothing must be persisted when the secret is rejected")
	require.Empty(t, *built)

	// Reusing backup credentials does not persist a secret, so it stays allowed.
	seedBackupS3(t, repo, BackupS3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "backup-bucket", AccessKeyID: "ak", SecretAccessKey: "sk", Prefix: "backups/",
	})
	_, err = svc.Update(ctx, ImageStorageSettings{Enabled: true, ReuseBackupS3: true})
	require.NoError(t, err)
}

func TestImageStorageSettingsIncompleteStaysDisabled(t *testing.T) {
	svc, _, built := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()

	_, err := svc.Update(ctx, ImageStorageSettings{Enabled: true, Bucket: "my-images"})
	require.NoError(t, err)

	_, enabled := svc.resolve()
	require.False(t, enabled, "missing credentials must not enable the feature")
	require.Empty(t, *built, "no client is built from an incomplete configuration")
}

// Deployments that already enabled the feature through config.yaml must keep
// working after the setting moves into the database.
func TestImageStorageSettingsFallBackToConfigFile(t *testing.T) {
	svc, _, built := newImageStorageFixture(t, config.ImageStorageConfig{
		Enabled: true, Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "yaml-bucket", AccessKeyID: "yaml-ak", SecretAccessKey: "yaml-sk",
		Prefix: "images/", MaxDownloadByte: 1024,
	})

	_, enabled := svc.resolve()
	require.True(t, enabled, "config.yaml still enables the feature when nothing is stored yet")
	require.Equal(t, "yaml-bucket", (*built)[0].Bucket)

	fetched, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.True(t, fetched.Enabled)
	require.Equal(t, "yaml-bucket", fetched.Bucket)
	require.Empty(t, fetched.SecretAccessKey)
	require.Equal(t, StorageProviderS3, fetched.Provider)
	require.NotNil(t, fetched.Resolved)
	require.Equal(t, "https://acct.r2.cloudflarestorage.com", fetched.Resolved.Endpoint)
}

func TestImageStorageSettingsReuseDoesNotPersistProviderOrSecret(t *testing.T) {
	svc, repo, built := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()
	seedBackupS3(t, repo, BackupS3Config{
		Provider: StorageProviderAliyunOSS, Region: "oss-cn-hangzhou",
		Bucket: "backup-bucket", AccessKeyID: "backup-ak", SecretAccessKey: "backup-sk",
		Prefix: "backups/", ForcePathStyle: true,
	})

	saved, err := svc.Update(ctx, ImageStorageSettings{
		Enabled: true, ReuseBackupS3: true, Bucket: "",
		Provider: StorageProviderQiniu, Endpoint: "https://image.example", Region: "cn-east-1",
		AccessKeyID: "image-ak", SecretAccessKey: "image-sk", ForcePathStyle: true,
		Resolved: &StorageResolvedEndpoint{Endpoint: "https://evil.example"},
	})
	require.NoError(t, err)
	require.Empty(t, saved.SecretAccessKey)
	require.Empty(t, saved.Endpoint)
	require.Empty(t, saved.AccessKeyID)
	require.Equal(t, StorageProviderAliyunOSS, saved.Provider)
	require.Equal(t, "https://s3.oss-cn-hangzhou.aliyuncs.com", saved.Resolved.Endpoint)
	require.Equal(t, "cn-hangzhou", saved.Resolved.Region)
	require.False(t, saved.Resolved.ForcePathStyle)

	raw, err := repo.GetValue(ctx, settingKeyImageStorageConfig)
	require.NoError(t, err)
	require.NotContains(t, raw, "qiniu")
	require.NotContains(t, raw, "aliyun_oss")
	require.NotContains(t, raw, "image-sk")
	require.NotContains(t, raw, "backup-sk")
	require.NotContains(t, raw, "image-ak")
	require.NotContains(t, raw, "backup-ak")
	require.NotContains(t, raw, "resolved")
	require.NotContains(t, raw, "s3.oss-cn-hangzhou")
	require.NotContains(t, raw, "evil.example")

	_, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, StorageProviderAliyunOSS, (*built)[0].Provider)
	require.Empty(t, (*built)[0].Endpoint)
	require.Equal(t, "oss-cn-hangzhou", (*built)[0].Region)
	require.Equal(t, "backup-sk", (*built)[0].SecretAccessKey)
	require.True(t, (*built)[0].ForcePathStyle)
	require.Equal(t, "backup-bucket", (*built)[0].Bucket)
}

func TestImageStorageSettingsOwnProviderResolvedIsNotStored(t *testing.T) {
	svc, repo, _ := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()

	saved, err := svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Provider: StorageProviderTencentCOS, Region: "ap-guangzhou",
		Bucket: "example-1250000000", AccessKeyID: "ak", SecretAccessKey: "super-secret",
		ForcePathStyle: true,
	})
	require.NoError(t, err)
	require.Empty(t, saved.SecretAccessKey)
	require.Empty(t, saved.Endpoint)
	require.Equal(t, StorageProviderTencentCOS, saved.Provider)
	require.Equal(t, "https://cos.ap-guangzhou.myqcloud.com", saved.Resolved.Endpoint)
	require.Equal(t, "ap-guangzhou", saved.Resolved.Region)
	require.False(t, saved.Resolved.ForcePathStyle)
	require.True(t, saved.ForcePathStyle)

	raw, err := repo.GetValue(ctx, settingKeyImageStorageConfig)
	require.NoError(t, err)
	require.Contains(t, raw, `"provider":"tencent_cos"`)
	require.NotContains(t, raw, "myqcloud.com")
	require.NotContains(t, raw, "resolved")
	require.NotContains(t, raw, `"secret_access_key":"super-secret"`)
	require.Contains(t, raw, `"secret_access_key":"enc:super-secret"`)

	_, err = svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Provider: StorageProviderAliyunOSS, Bucket: "images", AccessKeyID: "ak", SecretAccessKey: "sk",
	})
	require.ErrorContains(t, err, "region is required")
	_, err = svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Provider: StorageProviderAliyunOSS, Region: "auto", Bucket: "images", AccessKeyID: "ak", SecretAccessKey: "sk",
	})
	require.ErrorContains(t, err, "region is required")
	_, err = svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Provider: "minio", Region: "auto", Bucket: "images", AccessKeyID: "ak", SecretAccessKey: "sk",
	})
	require.ErrorContains(t, err, "unknown storage provider")
}

type headRecordingStorage struct {
	heads int
	saves int
}

func (s *headRecordingStorage) Save(_ context.Context, key, _ string, _ []byte) (string, error) {
	s.saves++
	return "https://cdn.example.com/" + key, nil
}

func (s *headRecordingStorage) HeadBucket(context.Context) error {
	s.heads++
	return nil
}

func TestImageStorageTestConnectionHeadBucketKeepsSecret(t *testing.T) {
	repo := newStubSettingRepo()
	encryptor := reversibleEncryptor{}
	backup := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}, encryptor, nil, nil)
	store := &headRecordingStorage{}
	var built []config.ImageStorageConfig
	factory := func(_ context.Context, cfg *config.ImageStorageConfig) (ImageStorage, error) {
		built = append(built, *cfg)
		return store, nil
	}
	svc := NewImageStorageSettingService(repo, encryptor, backup, factory, config.ImageStorageConfig{})
	ctx := context.Background()

	_, err := svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Provider: StorageProviderQiniu, Region: "cn-north-1",
		Bucket: "space", AccessKeyID: "ak", SecretAccessKey: "kept-secret",
	})
	require.NoError(t, err)

	resolved, err := svc.TestConnection(ctx, ImageStorageSettings{
		Enabled: true, Provider: StorageProviderQiniu, Region: "cn-north-1",
		Bucket: "space", AccessKeyID: "ak",
	})
	require.NoError(t, err)
	require.Equal(t, 1, store.heads)
	require.Zero(t, store.saves)
	require.Equal(t, "kept-secret", built[len(built)-1].SecretAccessKey)
	require.Empty(t, built[len(built)-1].Endpoint)
	require.Equal(t, "https://s3.cn-north-1.qiniucs.com", resolved.Endpoint)
	require.Equal(t, "cn-north-1", resolved.Region)
	require.False(t, resolved.ForcePathStyle)
	raw, marshalErr := json.Marshal(resolved)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(raw), "kept-secret")

	resolved, err = svc.TestConnection(ctx, ImageStorageSettings{
		Enabled: true, Provider: StorageProviderTencentCOS, Region: "ap-guangzhou",
		Bucket: "not-an-appid", AccessKeyID: "ak", SecretAccessKey: "kept-secret",
	})
	require.ErrorContains(t, err, "bucket must look like {name}-{appid}")
	require.Nil(t, resolved)
	require.Equal(t, 1, store.heads)

	resolved, err = svc.TestConnection(ctx, ImageStorageSettings{Enabled: true, Bucket: "only-bucket"})
	require.ErrorIs(t, err, ErrImageStorageIncomplete)
	require.NotNil(t, resolved)
	require.Empty(t, resolved.Endpoint)
	require.Equal(t, "auto", resolved.Region)
	require.Equal(t, 1, store.heads)
	raw, marshalErr = json.Marshal(resolved)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(raw), "kept-secret")

	seedBackupS3(t, repo, BackupS3Config{
		Provider: StorageProviderAliyunOSS, Region: "oss-cn-hangzhou",
		Bucket: "backup-bucket", AccessKeyID: "backup-ak", SecretAccessKey: "backup-sk",
	})
	resolved, err = svc.TestConnection(ctx, ImageStorageSettings{Enabled: true, ReuseBackupS3: true})
	require.NoError(t, err)
	require.Equal(t, "https://s3.oss-cn-hangzhou.aliyuncs.com", resolved.Endpoint)
	require.Equal(t, "cn-hangzhou", resolved.Region)
	require.False(t, resolved.ForcePathStyle)
	raw, marshalErr = json.Marshal(resolved)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(raw), "backup-sk")
}

func TestImageStorageUpdateEmptySecretDoesNotOverwriteWhenLoadFails(t *testing.T) {
	svc, repo, _ := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()

	_, err := svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Bucket: "my-images",
		Endpoint:    "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "ak", SecretAccessKey: "  super-secret  ",
	})
	require.NoError(t, err)
	raw, err := repo.GetValue(ctx, settingKeyImageStorageConfig)
	require.NoError(t, err)
	require.Contains(t, raw, "enc:super-secret")
	require.NotContains(t, raw, "enc:  super-secret")

	_, err = svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Bucket: "my-images",
		Endpoint: "https://acct.r2.cloudflarestorage.com", AccessKeyID: "ak",
		SecretAccessKey: " \t ",
	})
	require.NoError(t, err)
	require.True(t, svc.SecretConfigured(ctx))
	require.True(t, svc.OwnSecretConfigured(ctx))

	repo.getErr = errors.New("db down")
	_, err = svc.Update(ctx, ImageStorageSettings{
		Enabled: true, Bucket: "wiped",
		Endpoint: "https://acct.r2.cloudflarestorage.com", AccessKeyID: "ak",
	})
	require.Error(t, err)
	_, err = svc.TestConnection(ctx, ImageStorageSettings{
		Enabled: true, Bucket: "my-images",
		Endpoint: "https://acct.r2.cloudflarestorage.com", AccessKeyID: "ak",
	})
	require.Error(t, err)

	repo.getErr = nil
	rawAfter, err := repo.GetValue(ctx, settingKeyImageStorageConfig)
	require.NoError(t, err)
	require.Contains(t, rawAfter, `"secret_access_key":"enc:super-secret"`)
	require.NotContains(t, rawAfter, `"bucket":"wiped"`)
	require.True(t, svc.OwnSecretConfigured(ctx))
}

func TestImageStorageOwnSecretConfiguredIgnoresBackupSecretWhileReusing(t *testing.T) {
	svc, repo, _ := newImageStorageFixture(t, config.ImageStorageConfig{})
	ctx := context.Background()
	seedBackupS3(t, repo, BackupS3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto",
		Bucket: "backup-bucket", AccessKeyID: "backup-ak", SecretAccessKey: "backup-sk",
	})

	_, err := svc.Update(ctx, ImageStorageSettings{
		Enabled: true, ReuseBackupS3: true, Bucket: "images",
	})
	require.NoError(t, err)
	require.True(t, svc.SecretConfigured(ctx), "reuse reports the backup secret")
	require.False(t, svc.OwnSecretConfigured(ctx), "reuse does not copy the backup secret onto the image row")

	_, err = svc.Update(ctx, ImageStorageSettings{
		Enabled: true, ReuseBackupS3: false, Provider: StorageProviderS3, Region: "auto",
		Bucket: "images", Endpoint: "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "image-ak", SecretAccessKey: "image-sk",
	})
	require.NoError(t, err)
	require.True(t, svc.SecretConfigured(ctx))
	require.True(t, svc.OwnSecretConfigured(ctx))
}

func TestImageStorageFallbackRejectsAutoRegionForNonS3(t *testing.T) {
	providers := []string{StorageProviderAliyunOSS, StorageProviderTencentCOS, StorageProviderQiniu}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			svc, _, built := newImageStorageFixture(t, config.ImageStorageConfig{
				Enabled: true, Provider: "  " + provider + " ", Region: " auto ",
				Endpoint: "", Bucket: "example-1250000000", AccessKeyID: "ak", SecretAccessKey: "sk",
			})
			_, enabled := svc.resolve()
			require.False(t, enabled)
			require.Empty(t, *built, "a nonsense auto endpoint must not be handed to the client")

			_, err := svc.Get(context.Background())
			require.ErrorContains(t, err, "region is required")
			require.NotContains(t, err.Error(), "oss-auto")
			require.NotContains(t, err.Error(), "cos.auto")
			require.NotContains(t, err.Error(), "s3.auto")
		})
	}

	svc, _, built := newImageStorageFixture(t, config.ImageStorageConfig{
		Enabled: true, Provider: StorageProviderQiniu, Region: "cn-east-1",
		Bucket: "space", AccessKeyID: "ak", SecretAccessKey: "sk",
	})
	_, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, StorageProviderQiniu, (*built)[0].Provider)
	require.Equal(t, "cn-east-1", (*built)[0].Region)
	require.NotContains(t, (*built)[0].Endpoint, "auto")
}
