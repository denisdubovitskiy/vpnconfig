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

// newTestCachedLookup создаёт CachedIPLookup с тестовым моком и временным кешем.
func newTestCachedLookup(t *testing.T, cacheOpts ...Option) (*CachedIPLookup, *MockIPLookup, Storage) {
	t.Helper()

	cachePath := filepath.Join(t.TempDir(), "cache.json")
	mockLookup := NewMockIPLookup(t)
	storage := NewFileCache(cachePath, cacheOpts...)
	lookup := NewCachedIPLookup(mockLookup, storage)

	return lookup, mockLookup, storage
}

// writeCacheFile записывает тестовые данные в файл кеша в формате map.
func writeCacheFile(t *testing.T, storage Storage, data map[string]cacheEntry) {
	t.Helper()

	require.NoError(t, storage.Write(data))
}

func TestCachedIPLookup_CountryByIP(t *testing.T) {
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
		lookup, mockLookup, storage := newTestCachedLookup(t)
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
		loc, err := lookup.CountryByIP(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, testCountry, loc.Country)

		cacheData, err := storage.Read()
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
		lookup, mockLookup, storage := newTestCachedLookup(t)
		writeCacheFile(t, storage, map[string]cacheEntry{
			testIP: {
				Country:   cachedCountry,
				Timestamp: time.Now(),
			},
		})

		// act
		loc, err := lookup.CountryByIP(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, cachedCountry, loc.Country)
		mockLookup.AssertNotCalled(t, "CountryByIP")
	})

	// Проверяем, что устаревший кеш игнорируется и делается запрос к API.
	t.Run("expired cache triggers api request", func(t *testing.T) {
		t.Parallel()

		// arrange
		const newCountry = "NewCountry"
		lookup, mockLookup, storage := newTestCachedLookup(t, WithTTL(time.Hour))
		writeCacheFile(t, storage, map[string]cacheEntry{
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
		loc, err := lookup.CountryByIP(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, newCountry, loc.Country)
	})

	// Проверяем обработку ошибки от API.
	t.Run("api error is propagated", func(t *testing.T) {
		t.Parallel()

		// arrange
		wantErr := errors.New("api unavailable")
		lookup, mockLookup, _ := newTestCachedLookup(t)
		mockLookup.EXPECT().
			CountryByIP(mock.Anything, testIP).
			Return(nil, wantErr)

		// act
		_, err := lookup.CountryByIP(t.Context(), testIP)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)
	})

	// Проверяем автоопределение IP при пустом значении.
	t.Run("empty ip uses auto detection", func(t *testing.T) {
		t.Parallel()

		// arrange
		lookup, mockLookup, _ := newTestCachedLookup(t)
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
		loc, err := lookup.CountryByIP(t.Context(), "")

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, testAutoCountry, loc.Country)
	})

	// Проверяем, что кеш сохраняет старые записи при добавлении новых.
	t.Run("cache preserves existing entries", func(t *testing.T) {
		t.Parallel()

		// arrange
		const (
			oldIP      = "203.0.113.1"
			oldCountry = "Sweden"
		)
		lookup, mockLookup, storage := newTestCachedLookup(t)
		writeCacheFile(t, storage, map[string]cacheEntry{
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
		loc, err := lookup.CountryByIP(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, testCountry, loc.Country)

		cacheData, err := storage.Read()
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

func TestCachedIPLookup_CountryName(t *testing.T) {
	t.Parallel()

	const (
		testIP      = "192.0.2.1"
		testCountry = "Russia"
	)

	// Проверяем получение страны из кеша без запроса к API.
	t.Run("returns country from cache", func(t *testing.T) {
		t.Parallel()

		// arrange
		lookup, mockLookup, storage := newTestCachedLookup(t)
		writeCacheFile(t, storage, map[string]cacheEntry{
			testIP: {
				Country:   testCountry,
				Timestamp: time.Now(),
			},
		})

		// act
		country, err := lookup.CountryName(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		assert.Equal(t, testCountry, country)
		mockLookup.AssertNotCalled(t, "CountryByIP")
	})

	// Проверяем запрос к API при отсутствии кеша.
	t.Run("returns country from provider on cache miss", func(t *testing.T) {
		t.Parallel()

		// arrange
		lookup, mockLookup, _ := newTestCachedLookup(t)
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
		country, err := lookup.CountryName(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		assert.Equal(t, testCountry, country)
	})

	// Проверяем обработку ошибки от API.
	t.Run("propagates provider error", func(t *testing.T) {
		t.Parallel()

		// arrange
		wantErr := errors.New("api unavailable")
		lookup, mockLookup, _ := newTestCachedLookup(t)
		mockLookup.EXPECT().
			CountryByIP(mock.Anything, testIP).
			Return(nil, wantErr)

		// act
		country, err := lookup.CountryName(t.Context(), testIP)

		// assert
		assert.Empty(t, country)
		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)
	})
}
