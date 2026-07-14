package delivery_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/teamboard/services/domain/plugin/internal/delivery"
)

func TestComputeNextAttempt(t *testing.T) {
	t.Run("attempt 1 is ~30s", func(t *testing.T) {
		next, ok := delivery.ComputeNextAttempt(1)
		assert.True(t, ok)
		diff := time.Until(next)
		assert.Greater(t, diff, 20*time.Second)
		assert.Less(t, diff, 40*time.Second)
	})
	t.Run("attempt 7 is ~24h", func(t *testing.T) {
		next, ok := delivery.ComputeNextAttempt(7)
		assert.True(t, ok)
		diff := time.Until(next)
		assert.Greater(t, diff, 20*time.Hour)
		assert.Less(t, diff, 28*time.Hour)
	})
	t.Run("attempt 8 returns dead", func(t *testing.T) {
		_, ok := delivery.ComputeNextAttempt(8)
		assert.False(t, ok)
	})
}

func TestIsPermanentFailure(t *testing.T) {
	cases := []struct {
		code      int
		permanent bool
	}{
		{200, false},
		{500, false},
		{503, false},
		{408, false},
		{429, false},
		{400, true},
		{401, true},
		{403, true},
		{404, true},
		{410, true},
		{422, true},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.permanent, delivery.IsPermanentFailure(tc.code), "status %d", tc.code)
	}
}
