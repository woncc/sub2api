package service

import (
	"errors"
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// StorageProviderS3 is Cloudflare R2 and any other S3-compatible endpoint.
	StorageProviderS3 = "s3"
	// StorageProviderAliyunOSS is Alibaba Cloud OSS via its S3-compatible API.
	StorageProviderAliyunOSS = "aliyun_oss"
	// StorageProviderTencentCOS is Tencent Cloud COS.
	StorageProviderTencentCOS = "tencent_cos"
	// StorageProviderQiniu is Qiniu Kodo via its S3 endpoint.
	StorageProviderQiniu = "qiniu"
)

var (
	errUnknownStorageProvider = errors.New("unknown storage provider")
	errStorageRegionRequired  = errors.New("region is required")
	errTencentBucketAppID     = errors.New("bucket must look like {name}-{appid}")

	tencentBucketAppID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-[0-9]+$`)
)

// StorageResolvedEndpoint is the endpoint actually used to build an S3 client.
// It is computed for API responses and must not be stored in settings JSON.
type StorageResolvedEndpoint struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	ForcePathStyle bool   `json:"force_path_style"`
}

// ResolveStorageEndpoint derives the S3 endpoint, signing region, and path-style
// flag for a storage provider. Stored endpoint is a user override: an empty
// endpoint is derived, and a non-empty endpoint is returned unchanged.
// Empty provider is s3. The derived endpoint is never written back by this function.
func ResolveStorageEndpoint(provider, region, bucket, endpoint string, forcePathStyle bool) (resolvedEndpoint, signingRegion string, resolvedForcePathStyle bool, err error) {
	provider, err = canonicalStorageProvider(provider)
	if err != nil {
		return "", "", false, err
	}
	region = strings.TrimSpace(region)
	bucket = strings.TrimSpace(bucket)
	endpoint = strings.TrimSpace(endpoint)

	switch provider {
	case StorageProviderS3:
		return endpoint, region, forcePathStyle, nil
	case StorageProviderAliyunOSS:
		return resolveAliyunEndpoint(region, endpoint, forcePathStyle)
	case StorageProviderTencentCOS:
		return resolveTencentEndpoint(region, bucket, endpoint, forcePathStyle)
	case StorageProviderQiniu:
		return resolveQiniuEndpoint(region, endpoint, forcePathStyle)
	default:
		return "", "", false, errUnknownStorageProvider
	}
}

func canonicalStorageProvider(provider string) (string, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return StorageProviderS3, nil
	}
	switch provider {
	case StorageProviderS3, StorageProviderAliyunOSS, StorageProviderTencentCOS, StorageProviderQiniu:
		return provider, nil
	default:
		return "", errUnknownStorageProvider
	}
}

func resolveAliyunEndpoint(region, endpoint string, forcePathStyle bool) (string, string, bool, error) {
	if region == "" {
		return "", "", false, errStorageRegionRequired
	}
	if endpoint != "" {
		return endpoint, region, forcePathStyle, nil
	}
	regionID := strings.TrimPrefix(region, "oss-")
	if regionID == "" {
		return "", "", false, errStorageRegionRequired
	}
	return "https://s3.oss-" + regionID + ".aliyuncs.com", regionID, false, nil
}

func resolveTencentEndpoint(region, bucket, endpoint string, forcePathStyle bool) (string, string, bool, error) {
	if region == "" {
		return "", "", false, errStorageRegionRequired
	}
	if !tencentBucketAppID.MatchString(bucket) {
		return "", "", false, errTencentBucketAppID
	}
	if endpoint != "" {
		return endpoint, region, forcePathStyle, nil
	}
	return "https://cos." + region + ".myqcloud.com", region, false, nil
}

func resolveQiniuEndpoint(region, endpoint string, forcePathStyle bool) (string, string, bool, error) {
	if region == "" {
		return "", "", false, errStorageRegionRequired
	}
	if endpoint != "" {
		return endpoint, region, forcePathStyle, nil
	}
	return "https://s3." + region + ".qiniucs.com", region, false, nil
}

func resolvedStorageEndpoint(provider, region, bucket, endpoint string, forcePathStyle bool) (StorageResolvedEndpoint, error) {
	resolvedEndpoint, signingRegion, resolvedForcePathStyle, err := ResolveStorageEndpoint(provider, region, bucket, endpoint, forcePathStyle)
	if err != nil {
		return StorageResolvedEndpoint{}, err
	}
	return StorageResolvedEndpoint{
		Endpoint:       resolvedEndpoint,
		Region:         signingRegion,
		ForcePathStyle: resolvedForcePathStyle,
	}, nil
}

func invalidStorageConfig(err error) error {
	if err == nil {
		return nil
	}
	return infraerrors.BadRequest("INVALID_STORAGE_CONFIG", err.Error()).WithCause(err)
}

func isStorageConfigValidation(err error) bool {
	return errors.Is(err, errUnknownStorageProvider) ||
		errors.Is(err, errStorageRegionRequired) ||
		errors.Is(err, errTencentBucketAppID)
}
