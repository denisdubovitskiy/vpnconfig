package useragent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGenerator(t *testing.T) {
	t.Parallel()

	// Проверяем создание генератора с дефолтными значениями.
	t.Run("default generator", func(t *testing.T) {
		t.Parallel()

		// act
		gen := NewGenerator()

		// assert
		require.NotNil(t, gen)
		require.NotNil(t, gen.rand)
		assert.Empty(t, gen.userAgents)
	})

	// Проверяем создание генератора с кастомным списком User-Agent.
	t.Run("with user agents list", func(t *testing.T) {
		t.Parallel()

		// arrange
		const testUA1 = "Mozilla/1.0"
		const testUA2 = "Mozilla/2.0"
		agents := []string{testUA1, testUA2}

		// act
		gen := NewGenerator(WithUserAgentsList(agents))

		// assert
		require.NotNil(t, gen)
		assert.Equal(t, agents, gen.userAgents)
	})

	// Проверяем создание генератора с кастомным rand.
	t.Run("with custom rand", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockRand := NewMockRand(t)

		// act
		gen := NewGenerator(WithRand(mockRand))

		// assert
		require.NotNil(t, gen)
		assert.Equal(t, mockRand, gen.rand)
	})
}

func TestGenerator_RandomUserAgent(t *testing.T) {
	t.Parallel()

	// Проверяем возврат false при отсутствии User-Agent.
	t.Run("empty list returns false", func(t *testing.T) {
		t.Parallel()

		// arrange
		gen := NewGenerator()

		// act
		got, ok := gen.RandomUserAgent()

		// assert
		assert.False(t, ok)
		assert.Empty(t, got)
	})

	// Проверяем выбор User-Agent по индексу от rand.
	t.Run("selects user agent by rand index", func(t *testing.T) {
		t.Parallel()

		// arrange
		const (
			testUA1 = "Mozilla/1.0"
			testUA2 = "Mozilla/2.0"
			testUA3 = "Mozilla/3.0"
		)
		mockRand := NewMockRand(t)
		mockRand.EXPECT().
			IntN(3).
			Return(1)

		gen := NewGenerator(
			WithUserAgentsList([]string{testUA1, testUA2, testUA3}),
			WithRand(mockRand),
		)

		// act
		got, ok := gen.RandomUserAgent()

		// assert
		require.True(t, ok)
		assert.Equal(t, testUA2, got)
	})

	// Проверяем, что rand вызывается с правильным аргументом (длиной списка).
	t.Run("rand called with correct length", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockRand := NewMockRand(t)
		mockRand.EXPECT().
			IntN(5).
			Return(0)

		gen := NewGenerator(
			WithUserAgentsList(make([]string, 5)),
			WithRand(mockRand),
		)

		// act
		gen.RandomUserAgent()

		// assert — проверка выполнена через EXPECT
	})
}

func TestNewDefaultRandV2(t *testing.T) {
	t.Parallel()

	// Проверяем, что newDefaultRandV2 возвращает рабочий генератор.
	t.Run("returns non-nil rand", func(t *testing.T) {
		t.Parallel()

		// act
		r := newDefaultRandV2()

		// assert
		require.NotNil(t, r)

		// Проверяем, что IntN работает и не паникует.
		n := r.IntN(100)
		assert.GreaterOrEqual(t, n, 0)
		assert.Less(t, n, 100)
	})
}
