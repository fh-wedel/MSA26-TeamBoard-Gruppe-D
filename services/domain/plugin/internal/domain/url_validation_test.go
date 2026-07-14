package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/teamboard/services/domain/plugin/internal/domain"
)

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name              string
		url               string
		allowInsecureHTTP bool
		allowPrivate      bool
		allowedPorts      []int
		wantCode          string
	}{
		{"https allowed", "https://example.com/hook", false, true, nil, ""},
		{"http allowed when flag set", "http://example.com/hook", true, true, nil, ""},
		{"http rejected when flag not set", "http://example.com/hook", false, true, nil, "invalid_url"},
		{"ftp scheme rejected", "ftp://example.com/hook", false, true, nil, "invalid_url"},
		{"javascript scheme rejected", "javascript:alert(1)", false, true, nil, "invalid_url"},
		{"empty string rejected", "", false, true, nil, "invalid_url"},
		{"no host rejected", "https://", false, true, nil, "invalid_url"},
		{"port in allowlist", "https://example.com:8443/hook", false, true, []int{80, 443, 8080, 8443}, ""},
		{"port not in allowlist", "https://example.com:22/hook", false, true, []int{80, 443, 8080, 8443}, "invalid_url"},
		{"private URLs skipped when allowPrivate", "http://localhost/hook", true, true, nil, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := domain.ValidateWebhookURL(tc.url, tc.allowInsecureHTTP, tc.allowPrivate, tc.allowedPorts)
			if tc.wantCode == "" {
				assert.NoError(t, err)
			} else {
				var de *domain.Error
				assert.ErrorAs(t, err, &de)
				if de != nil {
					assert.Equal(t, tc.wantCode, de.Code)
				}
			}
		})
	}
}
