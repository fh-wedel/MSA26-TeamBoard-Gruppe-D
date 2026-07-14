package ordering_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/domain/task/internal/ordering"
)

func TestBetween_BothEmpty(t *testing.T) {
	pos, err := ordering.Between("", "")
	require.NoError(t, err)
	assert.NotEmpty(t, pos)
}

func TestBetween_AppendToEnd(t *testing.T) {
	first, _ := ordering.Between("", "")
	second, err := ordering.Between(first, "")
	require.NoError(t, err)
	assert.Greater(t, second, first, "second must be lexicographically greater than first")
}

func TestBetween_PrependToBeginning(t *testing.T) {
	first, _ := ordering.Between("", "")
	before, err := ordering.Between("", first)
	require.NoError(t, err)
	assert.Less(t, before, first, "prepended position must be less than first")
}

func TestBetween_Midpoint(t *testing.T) {
	cases := []struct{ prev, next string }{
		{"U", "V"},
		{"A", "Z"},
		{"0", "z"},
		{"U", "UZ"},
	}
	for _, tc := range cases {
		mid, err := ordering.Between(tc.prev, tc.next)
		require.NoError(t, err, "prev=%s next=%s", tc.prev, tc.next)
		assert.Greater(t, mid, tc.prev, "mid must be > prev (prev=%s next=%s mid=%s)", tc.prev, tc.next, mid)
		assert.Less(t, mid, tc.next, "mid must be < next (prev=%s next=%s mid=%s)", tc.prev, tc.next, mid)
	}
}

func TestBetween_Sequential(t *testing.T) {
	// Build a sequence and verify it sorts correctly.
	positions := make([]string, 10)
	pos, _ := ordering.Between("", "")
	positions[0] = pos
	for i := 1; i < 10; i++ {
		p, err := ordering.Between(positions[i-1], "")
		require.NoError(t, err)
		positions[i] = p
	}
	for i := 1; i < len(positions); i++ {
		assert.Greater(t, positions[i], positions[i-1], "position[%d] must be > position[%d]", i, i-1)
	}
}

func TestBetween_InsertInMiddle(t *testing.T) {
	a, _ := ordering.Between("", "")
	c, _ := ordering.Between(a, "")
	b, err := ordering.Between(a, c)
	require.NoError(t, err)
	assert.Greater(t, b, a)
	assert.Less(t, b, c)
}

func TestBetween_EqualPrevNext_Error(t *testing.T) {
	_, err := ordering.Between("U", "U")
	assert.Error(t, err)
}

func TestInitial(t *testing.T) {
	pos := ordering.Initial()
	assert.NotEmpty(t, pos)
}
