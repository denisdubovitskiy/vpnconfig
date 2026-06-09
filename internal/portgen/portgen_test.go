package portgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultGenerator_RandomPort(t *testing.T) {
	t.Parallel()

	// arrange
	g := New()

	// act
	port, err := g.RandomPort(t.Context())

	// assert
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	assert.Less(t, port, 65536)
}
