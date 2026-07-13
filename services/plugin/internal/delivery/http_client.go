package delivery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/teamboard/services/plugin/internal/domain"
)

// NewSSRFSafeClient returns an http.Client whose dialer rejects connections to
// private/special IP addresses (DNS-rebinding protection).
func NewSSRFSafeClient(connectTimeout, totalTimeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: connectTimeout,
		Control: ssrfControl,
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: totalTimeout,
	}

	client := &http.Client{
		Timeout:   totalTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Validate redirect target for SSRF.
			ips, err := net.LookupIP(req.URL.Hostname())
			if err != nil {
				return errors.New("redirect DNS resolution failed")
			}
			for _, ip := range ips {
				if domain.IsPrivateOrSpecialIP(ip) {
					return errors.New("redirect to private IP blocked")
				}
			}
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	return client
}

func ssrfControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil // let the dialer handle DNS errors
	}
	if domain.IsPrivateOrSpecialIP(ip) {
		return errors.New("connection to private IP blocked")
	}
	return nil
}

// Dummy context usage to satisfy import.
var _ = context.Background
