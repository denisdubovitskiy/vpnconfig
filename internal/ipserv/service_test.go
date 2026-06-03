package ipserv

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newTestService создаёт сервис с тестовым моком и временным кешем.
func newTestService(t *testing.T, cacheOpts ...Option) (*Service, *MockIPLookup, Cache) {
	t.Helper()

	cachePath := filepath.Join(t.TempDir(), "cache.json")
	mockLookup := NewMockIPLookup(t)
	cache := NewFileCache(cachePath, cacheOpts...)
	svc := NewService(mockLookup, cache)

	return svc, mockLookup, cache
}

// writeCacheFile записывает тестовые данные в файл кеша в формате map.
func writeCacheFile(t *testing.T, cache Cache, data map[string]cacheEntry) {
	t.Helper()

	require.NoError(t, cache.Write(data))
}

func TestService_CountryName(t *testing.T) {
	t.Parallel()

	const (
		testIP          = "192.0.2.1"
		testCountry     = "Russia"
		testAutoIP      = "198.51.100.1"
		testAutoCountry = "AutoCountry"
	)

	// Проверяем запрос к API при отсутствии кеша и сохранение результата.
	t.Run("api request when cache miss", func(t *testing.T) {
		t.Parallel()

		// arrange
		svc, mockLookup, cache := newTestService(t)
		mockLookup.EXPECT().
			CountryByIP(mock.Anything, testIP).
			Return(
				&Location{
					Status:  "success",
					Country: testCountry,
					Query:   testIP,
				},
				nil,
			)

		// act
		got, err := svc.CountryName(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		assert.Equal(t, testCountry, got)

		cacheData, err := cache.Read()
		require.NoError(t, err)

		entry, ok := cacheData[testIP]
		require.True(t, ok)
		assert.Equal(t, testCountry, entry.Country)
		assert.WithinDuration(t, time.Now(), entry.Timestamp, time.Minute)
	})

	// Проверяем чтение из актуального кеша без запроса к API.
	t.Run("cache hit returns cached value", func(t *testing.T) {
		t.Parallel()

		// arrange
		const cachedCountry = "CachedCountry"
		svc, mockLookup, cache := newTestService(t)
		writeCacheFile(t, cache, map[string]cacheEntry{
			testIP: {
				Country:   cachedCountry,
				Timestamp: time.Now(),
			},
		})

		// act
		got, err := svc.CountryName(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		assert.Equal(t, cachedCountry, got)
		mockLookup.AssertNotCalled(t, "CountryByIP")
	})

	// Проверяем, что устаревший кеш игнорируется и делается запрос к API.
	t.Run("expired cache triggers api request", func(t *testing.T) {
		t.Parallel()

		// arrange
		const newCountry = "NewCountry"
		svc, mockLookup, cache := newTestService(t, WithTTL(time.Hour))
		writeCacheFile(t, cache, map[string]cacheEntry{
			testIP: {
				Country:   "OldCountry",
				Timestamp: time.Now().Add(-2 * time.Hour),
			},
		})
		mockLookup.EXPECT().
			CountryByIP(mock.Anything, testIP).
			Return(
				&Location{
					Status:  "success",
					Country: newCountry,
					Query:   testIP,
				},
				nil,
			)

		// act
		got, err := svc.CountryName(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		assert.Equal(t, newCountry, got)
	})

	// Проверяем обработку ошибки от API.
	t.Run("api error is propagated", func(t *testing.T) {
		t.Parallel()

		// arrange
		wantErr := errors.New("api unavailable")
		svc, mockLookup, _ := newTestService(t)
		mockLookup.EXPECT().
			CountryByIP(mock.Anything, testIP).
			Return(nil, wantErr)

		// act
		_, err := svc.CountryName(t.Context(), testIP)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)
	})

	// Проверяем автоопределение IP при пустом значении.
	t.Run("empty ip uses auto detection", func(t *testing.T) {
		t.Parallel()

		// arrange
		svc, mockLookup, _ := newTestService(t)
		mockLookup.EXPECT().
			CountryByIP(mock.Anything, "").
			Return(
				&Location{
					Status:  "success",
					Country: testAutoCountry,
					Query:   testAutoIP,
				},
				nil,
			)

		// act
		got, err := svc.CountryName(t.Context(), "")

		// assert
		require.NoError(t, err)
		assert.Equal(t, testAutoCountry, got)
	})

	// Проверяем, что кеш сохраняет старые записи при добавлении новых.
	t.Run("cache preserves existing entries", func(t *testing.T) {
		t.Parallel()

		// arrange
		const (
			oldIP      = "203.0.113.1"
			oldCountry = "Sweden"
		)
		svc, mockLookup, cache := newTestService(t)
		writeCacheFile(t, cache, map[string]cacheEntry{
			oldIP: {
				Country:   oldCountry,
				Timestamp: time.Now(),
			},
		})
		mockLookup.EXPECT().
			CountryByIP(mock.Anything, testIP).
			Return(
				&Location{
					Status:  "success",
					Country: testCountry,
					Query:   testIP,
				},
				nil,
			)

		// act
		got, err := svc.CountryName(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		assert.Equal(t, testCountry, got)

		cacheData, err := cache.Read()
		require.NoError(t, err)

		// Проверяем, что старая запись сохранилась.
		oldEntry, ok := cacheData[oldIP]
		require.True(t, ok)
		assert.Equal(t, oldCountry, oldEntry.Country)

		// Проверяем, что новая запись добавилась.
		newEntry, ok := cacheData[testIP]
		require.True(t, ok)
		assert.Equal(t, testCountry, newEntry.Country)
	})
}
