package ipserv

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCache создаёт тестовый FileCache с фиксированным временем.
func newTestCache(t *testing.T, path string, now time.Time, opts ...Option) *FileCache {
	t.Helper()

	cache := NewFileCache(path, opts...)
	cache.nowFn = func() time.Time { return now }

	return cache
}

func TestFileCache_Read(t *testing.T) {
	t.Parallel()

	const (
		testIP      = "203.0.113.1"
		testCountry = "Sweden"
	)

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// Проверяем чтение несуществующего файла — должно вернуть пустую мапу.
	t.Run("missing file returns empty map", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "nonexistent.json")
		cache := newTestCache(t, cachePath, now)

		// act
		data, err := cache.Read()

		// assert
		require.NoError(t, err)
		assert.Empty(t, data)
	})

	// Проверяем чтение валидного кеша без TTL.
	t.Run("reads valid cache without ttl", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "cache.json")
		cache := newTestCache(t, cachePath, now)

		initial := map[string]cacheEntry{
			testIP: {
				Country:   testCountry,
				Timestamp: now.Add(-2 * time.Hour),
			},
		}
		require.NoError(t, cache.Write(initial))

		// act
		data, err := cache.Read()

		// assert
		require.NoError(t, err)
		require.Len(t, data, 1)
		assert.Equal(t, testCountry, data[testIP].Country)
	})

	// Проверяем, что при TTL возвращаются только актуальные записи.
	t.Run("filters expired entries with ttl", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "cache.json")
		cache := newTestCache(t, cachePath, now, WithTTL(time.Hour))

		initial := map[string]cacheEntry{
			testIP: {
				Country:   testCountry,
				Timestamp: now.Add(-2 * time.Hour),
			},
			"203.0.113.2": {
				Country:   "USA",
				Timestamp: now.Add(-30 * time.Minute),
			},
		}
		require.NoError(t, cache.Write(initial))

		// act
		data, err := cache.Read()

		// assert
		require.NoError(t, err)
		require.Len(t, data, 1)
		assert.Equal(t, "USA", data["203.0.113.2"].Country)
		assert.NotContains(t, data, testIP)
	})

	// Проверяем, что при TTL актуальные записи возвращаются.
	t.Run("returns fresh entries with ttl", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "cache.json")
		cache := newTestCache(t, cachePath, now, WithTTL(time.Hour))

		initial := map[string]cacheEntry{
			testIP: {
				Country:   testCountry,
				Timestamp: now.Add(-30 * time.Minute),
			},
		}
		require.NoError(t, cache.Write(initial))

		// act
		data, err := cache.Read()

		// assert
		require.NoError(t, err)
		require.Len(t, data, 1)
		assert.Equal(t, testCountry, data[testIP].Country)
	})
}

func TestFileCache_Write(t *testing.T) {
	t.Parallel()

	const (
		testIP      = "203.0.113.3"
		testCountry = "USA"
	)

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// Проверяем запись кеша в файл.
	t.Run("writes cache to file", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "cache.json")
		cache := newTestCache(t, cachePath, now)

		data := map[string]cacheEntry{
			testIP: {
				Country:   testCountry,
				Timestamp: now,
			},
		}

		// act
		err := cache.Write(data)

		// assert
		require.NoError(t, err)

		read, err := cache.Read()
		require.NoError(t, err)
		assert.Equal(t, testCountry, read[testIP].Country)
	})

	// Проверяем, что повторная перезапись обновляет данные.
	t.Run("overwrite updates data", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "cache.json")
		cache := newTestCache(t, cachePath, now)

		first := map[string]cacheEntry{
			"1.1.1.1": {Country: "First", Timestamp: now},
		}
		require.NoError(t, cache.Write(first))

		second := map[string]cacheEntry{
			"203.0.113.2": {Country: "Second", Timestamp: now},
		}

		// act
		err := cache.Write(second)

		// assert
		require.NoError(t, err)

		read, err := cache.Read()
		require.NoError(t, err)
		assert.Len(t, read, 1)
		assert.Equal(t, "Second", read["203.0.113.2"].Country)
	})

	// Проверяем, что Write очищает устаревшие записи при TTL.
	t.Run("cleans expired entries with ttl", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "cache.json")
		cache := newTestCache(t, cachePath, now, WithTTL(time.Hour))

		data := map[string]cacheEntry{
			testIP: {
				Country:   testCountry,
				Timestamp: now.Add(-2 * time.Hour),
			},
			"203.0.113.2": {
				Country:   "USA",
				Timestamp: now,
			},
		}

		// act
		err := cache.Write(data)

		// assert
		require.NoError(t, err)

		read, err := cache.Read()
		require.NoError(t, err)
		assert.Len(t, read, 1)
		assert.Equal(t, "USA", read["203.0.113.2"].Country)
		assert.NotContains(t, read, testIP)
	})
}

func TestFileCache_WithTTL(t *testing.T) {
	t.Parallel()

	// Проверяем, что опция WithTTL устанавливает TTL.
	t.Run("sets ttl option", func(t *testing.T) {
		t.Parallel()

		// arrange
		cachePath := filepath.Join(t.TempDir(), "cache.json")
		const wantTTL = 5 * time.Minute

		// act
		cache := NewFileCache(cachePath, WithTTL(wantTTL))

		// assert
		assert.Equal(t, wantTTL, cache.ttl)
	})
}
