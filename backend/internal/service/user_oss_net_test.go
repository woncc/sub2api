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
