package duration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDuration_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("valid duration string", func(t *testing.T) {
		t.Parallel()

		// arrange
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "90m"}
		var d Duration

		// act
		err := d.UnmarshalYAML(node)

		// assert
		require.NoError(t, err)
		assert.Equal(t, 90*time.Minute, d.Duration)
	})

	t.Run("valid seconds", func(t *testing.T) {
		t.Parallel()

		// arrange
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "10s"}
		var d Duration

		// act
		err := d.UnmarshalYAML(node)

		// assert
		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, d.Duration)
	})

	t.Run("invalid duration string", func(t *testing.T) {
		t.Parallel()

		// arrange
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "not-a-duration"}
		var d Duration

		// act
		err := d.UnmarshalYAML(node)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse duration")
	})

	t.Run("non-string node returns decode error", func(t *testing.T) {
		t.Parallel()

		// arrange
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		var d Duration

		// act
		err := d.UnmarshalYAML(node)

		// assert
		require.Error(t, err)
	})
}

func TestDuration_MarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("marshal hours", func(t *testing.T) {
		t.Parallel()

		// arrange
		d := Duration{Duration: 24 * time.Hour}

		// act
		got, err := d.MarshalYAML()

		// assert
		require.NoError(t, err)
		assert.Equal(t, "24h0m0s", got)
	})

	t.Run("marshal minutes", func(t *testing.T) {
		t.Parallel()

		// arrange
		d := Duration{Duration: 5 * time.Minute}

		// act
		got, err := d.MarshalYAML()

		// assert
		require.NoError(t, err)
		assert.Equal(t, "5m0s", got)
	})
}
