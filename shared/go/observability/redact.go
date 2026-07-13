package observability

import (
	"log/slog"
	"strings"
)

var sensitiveKeys = map[string]struct{}{
	"password": {}, "passwd": {}, "pass": {},
	"token": {}, "access_token": {}, "refresh_token": {},
	"secret": {}, "api_key": {}, "apikey": {},
	"authorization": {}, "auth": {},
	"private_key": {}, "private_key_pem": {},
	"key_encryption_key": {},
}

// RedactAttr replaces the value of known sensitive log attributes with "***".
func RedactAttr(_ []string, a slog.Attr) slog.Attr {
	if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, "***")
	}
	return a
}
