package delivery_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/teamboard/services/domain/plugin/internal/delivery"
)

func TestSignPayload(t *testing.T) {
	body := []byte(`{"event":"task.created"}`)
	sig := delivery.SignPayload("mysecret", body, 1715000000)
	assert.True(t, strings.HasPrefix(sig, "sha256="), "signature must start with sha256=")
	assert.Len(t, sig, 7+64) // "sha256=" + 64 hex chars

	// Deterministic for same inputs
	sig2 := delivery.SignPayload("mysecret", body, 1715000000)
	assert.Equal(t, sig, sig2)

	// Different secret → different sig
	sig3 := delivery.SignPayload("othersecret", body, 1715000000)
	assert.NotEqual(t, sig, sig3)
}
