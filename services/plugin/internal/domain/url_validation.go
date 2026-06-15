package domain

import (
	"net"
	"net/url"
	"strconv"
)

// ValidateWebhookURL checks that the given URL is safe to use as a webhook target.
// allowInsecureHTTP permits plain http:// (useful in dev/test).
// allowPrivate skips the private-IP check (useful in dev/test).
// allowedPorts is the set of allowed port numbers; empty means no port restriction.
func ValidateWebhookURL(rawURL string, allowInsecureHTTP, allowPrivate bool, allowedPorts []int) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ErrInvalidURL
	}

	switch u.Scheme {
	case "https":
		// always OK
	case "http":
		if !allowInsecureHTTP {
			return ErrInvalidURL
		}
	default:
		return ErrInvalidURL
	}

	if portStr := u.Port(); portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return ErrInvalidURL
		}
		if len(allowedPorts) > 0 && !containsPort(allowedPorts, p) {
			return ErrInvalidURL
		}
	}

	if allowPrivate {
		return nil
	}

	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return ErrInvalidURL
	}
	for _, ip := range ips {
		if isPrivateOrSpecial(ip) {
			return ErrPrivateURLForbidden
		}
	}
	return nil
}

func isPrivateOrSpecial(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.Equal(net.IPv4(169, 254, 169, 254)) ||
		ip.Equal(net.IPv4(100, 100, 100, 200))
}

func containsPort(ports []int, p int) bool {
	for _, allowed := range ports {
		if allowed == p {
			return true
		}
	}
	return false
}

// IsPrivateOrSpecialIP is exported for use by the delivery HTTP client.
func IsPrivateOrSpecialIP(ip net.IP) bool { return isPrivateOrSpecial(ip) }
