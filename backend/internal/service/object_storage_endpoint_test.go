//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveStorageEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		provider       string
		region         string
		bucket         string
		endpoint       string
		forcePathStyle bool
		wantEndpoint   string
		wantRegion     string
		wantPathStyle  bool
		wantErr        string
	}{
		{
			name:           "empty provider is s3 and passes endpoint through",
			provider:       "  ",
			region:         "  ",
			bucket:         " bucket ",
			endpoint:       "  https://acct.r2.cloudflarestorage.com  ",
			forcePathStyle: true,
			wantEndpoint:   "https://acct.r2.cloudflarestorage.com",
			wantRegion:     "",
			wantPathStyle:  true,
		},
		{
			name:          "s3 keeps empty region for newS3Client to default",
			provider:      "s3",
			region:        "",
			endpoint:      "",
			wantEndpoint:  "",
			wantRegion:    "",
			wantPathStyle: false,
		},
		{
			name:     "unknown provider",
			provider: "minio",
			region:   "auto",
			bucket:   "b",
			wantErr:  "unknown storage provider",
		},
		{
			name:     "provider match is case sensitive",
			provider: "Aliyun_OSS",
			region:   "cn-hangzhou",
			wantErr:  "unknown storage provider",
		},
		{
			name:     "aliyun requires region",
			provider: "aliyun_oss",
			region:   "  ",
			wantErr:  "region is required",
		},
		{
			name:           "aliyun derives hangzhou and ignores stored path style",
			provider:       " aliyun_oss ",
			region:         " cn-hangzhou ",
			bucket:         "example",
			forcePathStyle: true,
			wantEndpoint:   "https://s3.oss-cn-hangzhou.aliyuncs.com",
			wantRegion:     "cn-hangzhou",
			wantPathStyle:  false,
		},
		{
			name:          "aliyun strips one oss- prefix",
			provider:      "aliyun_oss",
			region:        "oss-cn-hangzhou",
			wantEndpoint:  "https://s3.oss-cn-hangzhou.aliyuncs.com",
			wantRegion:    "cn-hangzhou",
			wantPathStyle: false,
		},
		{
			name:          "aliyun strips only one leading oss-",
			provider:      "aliyun_oss",
			region:        "oss-oss-cn-hangzhou",
			wantEndpoint:  "https://s3.oss-oss-cn-hangzhou.aliyuncs.com",
			wantRegion:    "oss-cn-hangzhou",
			wantPathStyle: false,
		},
		{
			name:     "aliyun region that is only the prefix",
			provider: "aliyun_oss",
			region:   "oss-",
			wantErr:  "region is required",
		},
		{
			name:           "aliyun custom endpoint passes region and path style through",
			provider:       "aliyun_oss",
			region:         "oss-cn-hangzhou",
			endpoint:       " https://oss-cn-hangzhou.aliyuncs.com ",
			forcePathStyle: true,
			wantEndpoint:   "https://oss-cn-hangzhou.aliyuncs.com",
			wantRegion:     "oss-cn-hangzhou",
			wantPathStyle:  true,
		},
		{
			name:     "aliyun custom endpoint still requires region",
			provider: "aliyun_oss",
			endpoint: "https://oss-cn-hangzhou.aliyuncs.com",
			wantErr:  "region is required",
		},
		{
			name:     "tencent requires region",
			provider: "tencent_cos",
			bucket:   "example-1250000000",
			wantErr:  "region is required",
		},
		{
			name:           "tencent derives virtual-hosted endpoint",
			provider:       "tencent_cos",
			region:         " ap-guangzhou ",
			bucket:         " example-1250000000 ",
			forcePathStyle: true,
			wantEndpoint:   "https://cos.ap-guangzhou.myqcloud.com",
			wantRegion:     "ap-guangzhou",
			wantPathStyle:  false,
		},
		{
			name:     "tencent bucket must include appid",
			provider: "tencent_cos",
			region:   "ap-guangzhou",
			bucket:   "example",
			wantErr:  "bucket must look like {name}-{appid}",
		},
		{
			name:     "tencent bucket appid must be numeric",
			provider: "tencent_cos",
			region:   "ap-guangzhou",
			bucket:   "example-app",
			wantErr:  "bucket must look like {name}-{appid}",
		},
		{
			name:           "tencent custom endpoint keeps path style and bucket rule",
			provider:       "tencent_cos",
			region:         "ap-guangzhou",
			bucket:         "example-1250000000",
			endpoint:       " https://cos.ap-guangzhou.myqcloud.com ",
			forcePathStyle: true,
			wantEndpoint:   "https://cos.ap-guangzhou.myqcloud.com",
			wantRegion:     "ap-guangzhou",
			wantPathStyle:  true,
		},
		{
			name:     "tencent custom endpoint still checks bucket",
			provider: "tencent_cos",
			region:   "ap-guangzhou",
			bucket:   "example",
			endpoint: "https://example.internal",
			wantErr:  "bucket must look like {name}-{appid}",
		},
		{
			name:     "qiniu requires region and does not allowlist it",
			provider: "qiniu",
			bucket:   "space-name",
			wantErr:  "region is required",
		},
		{
			name:           "qiniu derives endpoint for any region id",
			provider:       "qiniu",
			region:         " cn-east-1 ",
			bucket:         "my-space",
			forcePathStyle: true,
			wantEndpoint:   "https://s3.cn-east-1.qiniucs.com",
			wantRegion:     "cn-east-1",
			wantPathStyle:  false,
		},
		{
			name:          "qiniu accepts a region outside the placeholder list",
			provider:      "qiniu",
			region:        "custom-1",
			wantEndpoint:  "https://s3.custom-1.qiniucs.com",
			wantRegion:    "custom-1",
			wantPathStyle: false,
		},
		{
			name:           "qiniu custom endpoint passes path style through",
			provider:       "qiniu",
			region:         "cn-east-1",
			endpoint:       " https://s3.internal.example ",
			forcePathStyle: true,
			wantEndpoint:   "https://s3.internal.example",
			wantRegion:     "cn-east-1",
			wantPathStyle:  true,
		},
		{
			name:          "s3 keeps the auto region sentinel",
			provider:      "s3",
			region:        " auto ",
			endpoint:      "https://acct.r2.cloudflarestorage.com",
			wantEndpoint:  "https://acct.r2.cloudflarestorage.com",
			wantRegion:    "auto",
			wantPathStyle: false,
		},
		{
			name:     "aliyun rejects the s3 auto sentinel",
			provider: "aliyun_oss",
			region:   " auto ",
			bucket:   "example",
			wantErr:  "region is required",
		},
		{
			name:     "aliyun auto sentinel is rejected even with a custom endpoint",
			provider: "aliyun_oss",
			region:   "auto",
			endpoint: "https://oss-cn-hangzhou.aliyuncs.com",
			wantErr:  "region is required",
		},
		{
			name:     "tencent rejects the s3 auto sentinel",
			provider: "tencent_cos",
			region:   "auto",
			bucket:   "example-1250000000",
			wantErr:  "region is required",
		},
		{
			name:     "qiniu rejects the s3 auto sentinel",
			provider: "qiniu",
			region:   "auto",
			bucket:   "space",
			wantErr:  "region is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotEndpoint, gotRegion, gotPathStyle, err := ResolveStorageEndpoint(tt.provider, tt.region, tt.bucket, tt.endpoint, tt.forcePathStyle)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantEndpoint, gotEndpoint)
			require.Equal(t, tt.wantRegion, gotRegion)
			require.Equal(t, tt.wantPathStyle, gotPathStyle)
		})
	}
}

func TestResolveStorageEndpointAliyunDerivationIsStable(t *testing.T) {
	t.Parallel()
	endpoint, region, pathStyle, err := ResolveStorageEndpoint(StorageProviderAliyunOSS, "oss-cn-hangzhou", "example", "", true)
	require.NoError(t, err)
	againEndpoint, againRegion, againPathStyle, err := ResolveStorageEndpoint(StorageProviderAliyunOSS, region, "example", endpoint, pathStyle)
	require.NoError(t, err)
	require.Equal(t, endpoint, againEndpoint)
	require.Equal(t, region, againRegion)
	require.Equal(t, pathStyle, againPathStyle)
	require.Equal(t, "https://s3.oss-cn-hangzhou.aliyuncs.com", againEndpoint)
	require.False(t, againPathStyle)
}
