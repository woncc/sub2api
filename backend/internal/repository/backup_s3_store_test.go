//go:build unit

package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestS3BackupStore_UploadFile(t *testing.T) {
	var received []byte
	var receivedLength int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		receivedLength = r.ContentLength
		var err error
		received, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := newS3Client(context.Background(), s3ClientParams{
		Endpoint:        server.URL,
		Region:          "auto",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		ForcePathStyle:  true,
	})
	require.NoError(t, err)

	content := []byte("streamed backup payload")
	filePath := t.TempDir() + "/part.gz"
	require.NoError(t, os.WriteFile(filePath, content, 0o600))

	store := &S3BackupStore{client: client, bucket: "backup-bucket"}
	size, err := store.UploadFile(context.Background(), "backup/part-1", filePath, "application/octet-stream")
	require.NoError(t, err)
	require.Equal(t, int64(len(content)), size)
	require.Equal(t, int64(len(content)), receivedLength)
	require.Equal(t, content, received)
}

func TestS3ClientUsesResolvedEndpoint(t *testing.T) {
	ctx := context.Background()
	factory := NewS3BackupStoreFactory()

	backupCfg := &service.BackupS3Config{
		Provider:        service.StorageProviderAliyunOSS,
		Region:          "oss-cn-hangzhou",
		Bucket:          " example ",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		ForcePathStyle:  true,
	}
	store, err := factory(ctx, backupCfg)
	require.NoError(t, err)
	require.Empty(t, backupCfg.Endpoint, "derived endpoint must not be written back onto the stored config")
	require.True(t, backupCfg.ForcePathStyle, "stored path-style flag is kept; derivation only affects the client")

	opts := store.(*S3BackupStore).client.Options()
	require.Equal(t, "https://s3.oss-cn-hangzhou.aliyuncs.com", aws.ToString(opts.BaseEndpoint))
	require.Equal(t, "cn-hangzhou", opts.Region)
	require.False(t, opts.UsePathStyle)
	require.Equal(t, aws.RequestChecksumCalculationWhenRequired, opts.RequestChecksumCalculation)
	require.NotEmpty(t, opts.APIOptions)
	require.Equal(t, "example", store.(*S3BackupStore).bucket)

	image, err := NewS3ImageStorage(ctx, &config.ImageStorageConfig{
		Provider:        service.StorageProviderQiniu,
		Region:          "cn-east-1",
		Bucket:          "image-space",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		ForcePathStyle:  true,
	})
	require.NoError(t, err)
	imageOpts := image.client.Options()
	require.Equal(t, "https://s3.cn-east-1.qiniucs.com", aws.ToString(imageOpts.BaseEndpoint))
	require.Equal(t, "cn-east-1", imageOpts.Region)
	require.False(t, imageOpts.UsePathStyle)

	_, err = factory(ctx, &service.BackupS3Config{
		Provider:        service.StorageProviderAliyunOSS,
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		Bucket:          "example",
	})
	require.ErrorContains(t, err, "region is required")

	s3Store, err := factory(ctx, &service.BackupS3Config{
		Endpoint:        "https://acct.r2.cloudflarestorage.com",
		Bucket:          "backup-bucket",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		ForcePathStyle:  true,
	})
	require.NoError(t, err)
	s3Opts := s3Store.(*S3BackupStore).client.Options()
	require.Equal(t, "https://acct.r2.cloudflarestorage.com", aws.ToString(s3Opts.BaseEndpoint))
	require.Equal(t, "auto", s3Opts.Region)
	require.True(t, s3Opts.UsePathStyle)
}

func TestS3ImageStorageHeadBucketDoesNotWrite(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		if r.Method != http.MethodHead {
			t.Errorf("image connection test wrote via %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store, err := NewS3ImageStorage(context.Background(), &config.ImageStorageConfig{
		Provider:        service.StorageProviderS3,
		Endpoint:        server.URL,
		Region:          "auto",
		Bucket:          "image-bucket",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		ForcePathStyle:  true,
	})
	require.NoError(t, err)
	require.NoError(t, store.HeadBucket(context.Background()))
	require.Equal(t, http.MethodHead, method)
	require.Contains(t, path, "image-bucket")
}
