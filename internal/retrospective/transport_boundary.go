// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// serverOrigin validates a credential-safe base without echoing untrusted URLs.
// Spec: docs/specs/task-retrospective.md via HTTPSessionClient
func serverOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(raw, "?#") || u.RawQuery != "" || u.ForceQuery || (u.Path != "" && u.Path != "/") || u.Opaque != "" || strings.HasSuffix(u.Host, ":") {
		return "", errors.New("invalid server base URL; require HTTP(S) origin without userinfo, query, fragment or path")
	}
	host := strings.ToLower(u.Hostname())
	if strings.ContainsAny(host, "% \\\t\r\n") {
		return "", errors.New("invalid server hostname")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("invalid server port")
	}
	return u.Scheme + "|" + host + "|" + strconv.Itoa(n), nil
}

// collectionServerURL validates collection permission, distinct from auth authority.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func collectionServerURL(raw string, cfg *Config) bool {
	if _, err := serverOrigin(raw); err != nil {
		return false
	}
	u, _ := url.Parse(raw)
	host := strings.ToLower(u.Hostname())
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	if cfg != nil && cfg.DispatchServer != "" {
		if _, err := serverOrigin(cfg.DispatchServer); err == nil {
			trusted, _ := url.Parse(cfg.DispatchServer)
			return strings.EqualFold(host, trusted.Hostname())
		}
	}
	return false
}

// trustedServerURL binds credentials to scheme, hostname and effective port.
// Spec: docs/specs/task-retrospective.md via reconcileServer
func trustedServerURL(raw, trusted string) bool {
	origin, err := serverOrigin(raw)
	if err != nil {
		return false
	}
	expected, err := serverOrigin(trusted)
	return err == nil && origin == expected
}
