package service

import (
	"context"
	"net"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewUserOSSHTTPClientIgnoresEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")
	t.Setenv("NO_PROXY", "")
	t.Setenv("http_proxy", "http://proxy.example:8080")
	t.Setenv("https_proxy", "http://proxy.example:8080")
	t.Setenv("no_proxy", "")

	client := NewUserOSSHTTPClient()
	roundTripper, ok := client.Transport.(userOSSRoundTripper)
	require.True(t, ok)
	transport, ok := roundTripper.base.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.Proxy)
	require.Equal(t, reflect.ValueOf(dialUserOSSPublic).Pointer(), reflect.ValueOf(transport.DialContext).Pointer())

	// DefaultTransport.Proxy is ProxyFromEnvironment. That function caches the
	// first process environment, so a later Setenv cannot prove it would select
	// a proxy. The client must drop the function entirely.
	base, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, base.Proxy)
}

func TestRejectUserOSSIPBlocksMonitorOnlyRanges(t *testing.T) {
	blocked := []string{
		"100.100.100.200",
		"100.64.0.0",
		"100.64.0.1",
		"100.127.255.255",
		"0.0.0.0",
		"0.0.0.1",
		"0.1.2.3",
		"0.255.255.255",
		"::ffff:100.100.100.200",
		"::ffff:0.1.2.3",
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		require.NotNil(t, ip, raw)
		require.ErrorContains(t, rejectUserOSSIP(ip), "resolved address is not allowed", raw)
	}

	allowed := []string{
		"1.1.1.1",
		"8.8.8.8",
		"100.63.255.255",
		"100.128.0.1",
		"1.0.0.1",
	}
	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		require.NotNil(t, ip, raw)
		require.NoError(t, rejectUserOSSIP(ip), raw)
	}
}

func TestParseLegacyIPv4(t *testing.T) {
	// 100.100.100.200 is 1684301000 decimal. 1684300999 is 100.100.100.199.
	aliyunMeta := net.ParseIP("100.100.100.200").To4()
	require.NotNil(t, aliyunMeta)
	decimalAliyun := uint32(aliyunMeta[0])<<24 | uint32(aliyunMeta[1])<<16 | uint32(aliyunMeta[2])<<8 | uint32(aliyunMeta[3])
	require.Equal(t, uint32(1684301000), decimalAliyun)

	cases := []struct {
		raw  string
		want string
	}{
		{"2130706433", "127.0.0.1"},
		{"0x7f000001", "127.0.0.1"},
		{"0X7F000001", "127.0.0.1"},
		{"0177.0.0.1", "127.0.0.1"},
		{"127.1", "127.0.0.1"},
		{"127.0.1", "127.0.0.1"},
		{"0x7f.1", "127.0.0.1"},
		{"017700000001", "127.0.0.1"},
		{"10.1", "10.0.0.1"},
		{"0xa9fea9fe", "169.254.169.254"},
		{"1684300999", "100.100.100.199"},
		{"1684301000", "100.100.100.200"},
		{"100.100.100.200", "100.100.100.200"},
	}
	for _, tc := range cases {
		ip, ok := parseLegacyIPv4(tc.raw)
		require.True(t, ok, tc.raw)
		require.Equal(t, tc.want, ip.String(), tc.raw)
	}

	for _, raw := range []string{
		"s3.amazonaws.com",
		"0x.example.com",
		"256.1.1.1",
		"1.2.3.4.5",
		"0x",
		"08",
		"4294967296",
		"",
		"1..2",
	} {
		_, ok := parseLegacyIPv4(raw)
		require.False(t, ok, raw)
	}
}

func TestDialUserOSSPublicRejectsLegacyIPv4LiteralsWithoutDNS(t *testing.T) {
	// These forms are IP literals, so dialUserOSSPublic must not call the
	// resolver. The assertion holds with GODEBUG=netdns=cgo and with the pure
	// Go resolver: a cancelled context would surface as a lookup error if DNS
	// ran, and the error text is the SSRF rejection instead.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, addr := range []string{
		"2130706433:9",
		"0x7f000001:9",
		"0X7F000001:9",
		"0177.0.0.1:9",
		"127.1:9",
		"127.0.1:9",
		"0x7f.1:9",
		"017700000001:9",
		"10.1:443",
		"0xa9fea9fe:80",
		"1684300999:443",
		"1684301000:443",
	} {
		_, err := dialUserOSSPublic(ctx, "tcp", addr)
		require.ErrorContains(t, err, "resolved address is not allowed", addr)
		require.NotContains(t, err.Error(), "lookup", addr)
	}
}

func TestValidateUserOSSRejectsLegacyIPv4Literals(t *testing.T) {
	for _, endpoint := range []string{
		"https://2130706433",
		"https://0x7f000001",
		"https://127.1",
		"https://0177.0.0.1",
	} {
		require.ErrorContains(t, validateUserOSSEndpoint(endpoint), "endpoint must be an https URL with a public host", endpoint)
	}
	require.NoError(t, validateUserOSSEndpoint("https://s3.us-east-1.amazonaws.com"))

	require.ErrorContains(t, validateUserOSSFetchURL("https://2130706433/bucket"), "artifact url host is not allowed")
	require.ErrorContains(t, validateUserOSSFetchURL("https://0x7f.1/latest"), "artifact url host is not allowed")

	require.ErrorContains(t, artifactURLAllowed("https://cdn.example", "https://127.1/a.png"), "blocked host")
	require.ErrorContains(t, artifactURLAllowed("https://cdn.example", "https://0xa9fea9fe/a.png"), "blocked host")
}

func TestDialUserOSSPublicRejectsCGNATAndThisNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, addr := range []string{
		"100.100.100.200:443",
		"100.64.0.1:443",
		"100.127.255.255:443",
		"0.0.0.1:443",
		"0.255.255.255:9",
		"[::ffff:100.100.100.200]:443",
		"[::ffff:0.1.2.3]:443",
	} {
		_, err := dialUserOSSPublic(ctx, "tcp", addr)
		require.ErrorContains(t, err, "resolved address is not allowed", addr)
	}
}
