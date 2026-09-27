package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// PublicUserOSSCheckMessage is the only connection-test text returned to the client.
// SDK and decrypt failures stay on the server so a bucket error cannot echo secrets.
func PublicUserOSSCheckMessage(err error) string {
	if err == nil {
		return "connection successful"
	}
	var app *infraerrors.ApplicationError
	if errors.As(err, &app) && app != nil && app.Code > 0 && app.Code < 500 && strings.TrimSpace(app.Message) != "" {
		return app.Message
	}
	return "connection failed"
}

func urlvalidatorBlockedHost(host string) bool {
	return urlvalidator.IsBlockedHost(host) || isUserOSSMetadataHost(host)
}

func isUserOSSMetadataHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	switch host {
	case "metadata", "metadata.goog", "metadata.google.internal":
		return true
	default:
		return strings.HasSuffix(host, ".metadata.google.internal") || strings.HasSuffix(host, ".metadata.goog")
	}
}

func validateUserOSSEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	if strings.ContainsAny(endpoint, "\\ \t\r\n") {
		return errors.New("endpoint must be an https URL with a public host")
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(endpoint, urlvalidator.ValidationOptions{})
	if err != nil {
		return errors.New("endpoint must be an https URL with a public host")
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("endpoint must be an https URL with a public host")
	}
	if path := strings.Trim(parsed.EscapedPath(), "/"); path != "" {
		return errors.New("endpoint must not include a path")
	}
	if isUserOSSMetadataHost(parsed.Hostname()) {
		return errors.New("endpoint must be an https URL with a public host")
	}
	return nil
}

func validateUserOSSFetchURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("artifact url is not allowed")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("artifact url must be https")
	}
	if urlvalidatorBlockedHost(parsed.Hostname()) {
		return errors.New("artifact url host is not allowed")
	}
	return nil
}

// artifactURLAllowed reports whether a URL handed back to the caller stays on the
// repository domain. Host comparison is exact so a prefix such as https://cdn.example
// does not accept https://cdn.example.evil.
func artifactURLAllowed(publicBase, target string) error {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("object storage returned an invalid url")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("object storage returned an invalid url")
	}
	if urlvalidatorBlockedHost(parsed.Hostname()) {
		return errors.New("object storage returned a url on a blocked host")
	}
	publicBase = strings.TrimSpace(publicBase)
	if publicBase == "" {
		return nil
	}
	base, err := url.Parse(publicBase)
	if err != nil || base.Host == "" || !strings.EqualFold(base.Scheme, parsed.Scheme) ||
		!strings.EqualFold(base.Hostname(), parsed.Hostname()) || base.Port() != parsed.Port() {
		return errors.New("object storage did not return a URL on the configured domain")
	}
	basePath := strings.Trim(base.EscapedPath(), "/")
	targetPath := strings.Trim(parsed.EscapedPath(), "/")
	if basePath != "" && targetPath != basePath && !strings.HasPrefix(targetPath, basePath+"/") {
		return errors.New("object storage did not return a URL on the configured domain")
	}
	return nil
}

func rejectUserOSSIP(ip net.IP) error {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return errors.New("resolved address is not allowed")
	}
	// Classification above covers RFC1918, loopback, and link-local. The
	// channel-monitor list also blocks CGNAT (100.64.0.0/10, including Aliyun
	// metadata 100.100.100.200) and 0.0.0.0/8. One blocked answer still fails
	// the whole lookup in dialUserOSSPublic.
	for _, network := range monitorBlockedCIDRs {
		if network.Contains(ip) {
			return errors.New("resolved address is not allowed")
		}
	}
	return nil
}

func dialUserOSSPublic(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if urlvalidatorBlockedHost(host) {
		return nil, errors.New("host is not allowed")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("host did not resolve")
	}
	for _, ip := range ips {
		if err := rejectUserOSSIP(ip.IP); err != nil {
			return nil, err
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var dialErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		dialErr = err
	}
	if dialErr == nil {
		dialErr = errors.New("dial failed")
	}
	return nil, dialErr
}

type userOSSRoundTripper struct {
	base http.RoundTripper
}

func (t userOSSRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("artifact url is not allowed")
	}
	if err := validateUserOSSFetchURL(req.URL.String()); err != nil {
		return nil, err
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// NewUserOSSHTTPClient is the outbound client for a user's bucket and for
// downloading an upstream artifact before that upload. It refuses cleartext,
// metadata hosts, and private or link-local addresses, including after DNS.
// It dials the target directly: an environment proxy would be the dial
// destination, so the target's resolved addresses would never be checked.
func NewUserOSSHTTPClient() *http.Client {
	base, _ := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if base != nil {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	// DefaultTransport.Proxy is ProxyFromEnvironment. Leave it unset, matching
	// newSSRFSafeHTTPClient, so HeadBucket, PutObject, and artifact downloads
	// cannot skip dialUserOSSPublic by connecting to HTTP_PROXY/HTTPS_PROXY.
	transport.Proxy = nil
	transport.DialContext = dialUserOSSPublic
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 60 * time.Second
	return &http.Client{
		Transport: userOSSRoundTripper{base: transport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return errors.New("redirect is not allowed")
			}
			if req == nil || req.URL == nil {
				return errors.New("artifact url is not allowed")
			}
			return validateUserOSSFetchURL(req.URL.String())
		},
	}
}
