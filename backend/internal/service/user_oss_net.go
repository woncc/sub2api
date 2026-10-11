package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
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
	if isUserOSSMetadataHost(parsed.Hostname()) || userOSSLegacyIPv4Literal(parsed.Hostname()) {
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
	if urlvalidatorBlockedHost(parsed.Hostname()) || userOSSLegacyIPv4Literal(parsed.Hostname()) {
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
	if urlvalidatorBlockedHost(parsed.Hostname()) || userOSSLegacyIPv4Literal(parsed.Hostname()) {
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
	// IP literals never reach DNS. Go's resolver only accepts the forms
	// net.ParseIP understands; glibc inet_aton / getaddrinfo also accept
	// decimal, hex, octal, and fewer than four dotted parts. Fold those
	// first so a cgo resolver cannot turn them into a private address.
	ips := userOSSLiteralAddrs(host)
	if ips == nil {
		ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
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

// userOSSLiteralAddrs returns the address when host is an IP literal, including
// glibc inet_aton forms that net.ParseIP rejects. A nil result means the host
// is a name and must be resolved.
func userOSSLiteralAddrs(host string) []net.IPAddr {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}
	}
	if ip, ok := parseLegacyIPv4(host); ok {
		return []net.IPAddr{{IP: ip}}
	}
	return nil
}

// userOSSLegacyIPv4Literal reports hosts that glibc accepts as IPv4 but that
// are not the canonical dotted form Go's net.ParseIP accepts. Canonical
// literals stay on the existing IsBlockedHost / rejectUserOSSIP path.
func userOSSLegacyIPv4Literal(host string) bool {
	host = strings.TrimSpace(host)
	ip, ok := parseLegacyIPv4(host)
	if !ok {
		return false
	}
	parsed := net.ParseIP(host)
	return parsed == nil || parsed.String() != ip.String()
}

// parseLegacyIPv4 implements glibc inet_aton: one to four dot-separated parts,
// each decimal, 0-prefixed octal, or 0x/0X hex. The last part fills the
// remaining bytes (one part is 32 bits, two is a.24, three is a.b.16). A
// trailing dot, empty part, overflow, or any other character fails. Names
// that contain letters outside a hex part, such as s3.amazonaws.com, fail.
func parseLegacyIPv4(host string) (net.IP, bool) {
	if host == "" || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return nil, false
	}
	parts := strings.Split(host, ".")
	if len(parts) < 1 || len(parts) > 4 {
		return nil, false
	}
	values := make([]uint64, len(parts))
	for i, part := range parts {
		v, ok := parseInetAtonPart(part)
		if !ok {
			return nil, false
		}
		values[i] = v
	}
	var maxLast uint64
	switch len(parts) {
	case 1:
		maxLast = 0xffffffff
	case 2:
		maxLast = 0xffffff
	case 3:
		maxLast = 0xffff
	default:
		maxLast = 0xff
	}
	for i, v := range values {
		limit := uint64(0xff)
		if i == len(values)-1 {
			limit = maxLast
		}
		if v > limit {
			return nil, false
		}
	}
	var addr uint32
	switch len(values) {
	case 1:
		addr = uint32(values[0])
	case 2:
		addr = uint32(values[0])<<24 | uint32(values[1])
	case 3:
		addr = uint32(values[0])<<24 | uint32(values[1])<<16 | uint32(values[2])
	default:
		addr = uint32(values[0])<<24 | uint32(values[1])<<16 | uint32(values[2])<<8 | uint32(values[3])
	}
	return net.IPv4(byte(addr>>24), byte(addr>>16), byte(addr>>8), byte(addr)), true
}

func parseInetAtonPart(part string) (uint64, bool) {
	if part == "" {
		return 0, false
	}
	base := 10
	digits := part
	if len(part) > 1 && part[0] == '0' {
		if part[1] == 'x' || part[1] == 'X' {
			base = 16
			digits = part[2:]
			if digits == "" {
				return 0, false
			}
		} else {
			base = 8
			digits = part[1:]
		}
	}
	for _, r := range digits {
		switch {
		case r >= '0' && r <= '9':
			if base == 8 && r > '7' {
				return 0, false
			}
		case base == 16 && ((r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')):
		default:
			return 0, false
		}
	}
	v, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, false
	}
	return v, true
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
