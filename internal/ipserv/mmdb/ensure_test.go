package mmdb

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
)

func TestEnsureDatabase(t *testing.T) {
	t.Parallel()

	t.Run("skips download if file exists and is non-empty", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.mmdb")
		require.NoError(t, os.WriteFile(dbPath, []byte("existing-content"), 0o644))
		cfg := newMMDBConfig(dbPath, "")

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, http.DefaultClient)

		// assert
		require.NoError(t, err)
		data, err := os.ReadFile(dbPath)
		require.NoError(t, err)
		assert.Equal(t, "existing-content", string(data))
	})

	t.Run("downloads when file does not exist", func(t *testing.T) {
		t.Parallel()

		// arrange
		server := newTestServerWithPayload(t, testMMDBContent)
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.mmdb")
		cfg := newMMDBConfig(dbPath, server.URL)

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, server.Client())

		// assert
		require.NoError(t, err)
		data, err := os.ReadFile(dbPath)
		require.NoError(t, err)
		assert.Equal(t, testMMDBContent, string(data))
	})

	t.Run("creates all parent directories", func(t *testing.T) {
		t.Parallel()

		// arrange
		server := newTestServerWithPayload(t, "ok")
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "a", "b", "c", "d", "test.mmdb")
		cfg := newMMDBConfig(dbPath, server.URL)

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, server.Client())

		// assert
		require.NoError(t, err)
		info, err := os.Stat(dbPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	})

	t.Run("re-downloads when existing file is empty", func(t *testing.T) {
		t.Parallel()

		// arrange
		server := newTestServerWithPayload(t, "fresh-content")
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.mmdb")
		require.NoError(t, os.WriteFile(dbPath, nil, 0o644))
		cfg := newMMDBConfig(dbPath, server.URL)

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, server.Client())

		// assert
		require.NoError(t, err)
		data, err := os.ReadFile(dbPath)
		require.NoError(t, err)
		assert.Equal(t, "fresh-content", string(data))
	})

	t.Run("propagates error on non-2xx status", func(t *testing.T) {
		t.Parallel()

		// arrange
		server := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.mmdb")
		cfg := newMMDBConfig(dbPath, server.URL)

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, server.Client())

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
		// Файл не должен был остаться.
		_, statErr := os.Stat(dbPath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("re-downloads when file is older than max_age", func(t *testing.T) {
		t.Parallel()

		// arrange
		var downloadCount atomic.Int32
		server := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			downloadCount.Add(1)
			_, _ = w.Write([]byte("fresh-content"))
		}))
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.mmdb")
		writeStaleFile(t, dbPath, "old-content", 48*time.Hour)
		cfg := newMMDBConfig(dbPath, server.URL, withMaxAge(24*time.Hour))

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, server.Client())

		// assert
		require.NoError(t, err)
		assert.Equal(t, int32(1), downloadCount.Load())
		data, err := os.ReadFile(dbPath)
		require.NoError(t, err)
		assert.Equal(t, "fresh-content", string(data))
	})

	t.Run("skips download when file is within max_age", func(t *testing.T) {
		t.Parallel()

		// arrange
		var downloadCount atomic.Int32
		server := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			downloadCount.Add(1)
			_, _ = w.Write([]byte("unwanted"))
		}))
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.mmdb")
		writeStaleFile(t, dbPath, "recent-content", 1*time.Hour)
		cfg := newMMDBConfig(dbPath, server.URL, withMaxAge(24*time.Hour))

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, server.Client())

		// assert
		require.NoError(t, err)
		assert.Equal(t, int32(0), downloadCount.Load())
		data, err := os.ReadFile(dbPath)
		require.NoError(t, err)
		assert.Equal(t, "recent-content", string(data))
	})

	t.Run("max_age = 0 disables age check", func(t *testing.T) {
		t.Parallel()

		// arrange
		var downloadCount atomic.Int32
		server := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			downloadCount.Add(1)
			_, _ = w.Write([]byte("unwanted"))
		}))
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.mmdb")
		// Файл очень старый, но max_age = 0 означает "проверка возраста отключена".
		writeStaleFile(t, dbPath, "old-but-valid", 1000*time.Hour)
		cfg := newMMDBConfig(dbPath, server.URL, withMaxAge(0))

		// act
		err := ensureDatabase(logger.IntoContext(t.Context(), logger.Silent()), cfg, server.Client())

		// assert
		require.NoError(t, err)
		assert.Equal(t, int32(0), downloadCount.Load())
		data, err := os.ReadFile(dbPath)
		require.NoError(t, err)
		assert.Equal(t, "old-but-valid", string(data))
	})
}

func TestDownloadFile(t *testing.T) {
	t.Parallel()

	t.Run("writes file atomically via temp + rename", func(t *testing.T) {
		t.Parallel()

		// arrange
		server := newTestServerWithPayload(t, "payload")
		dir := t.TempDir()
		destPath := filepath.Join(dir, "nested", "out.bin")

		// act
		err := downloadFile(t.Context(), server.URL, destPath, server.Client())

		// assert
		require.NoError(t, err)
		data, err := os.ReadFile(destPath)
		require.NoError(t, err)
		assert.Equal(t, "payload", string(data))
		// Временный файл не должен остаться.
		_, statErr := os.Stat(destPath + ".tmp")
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("removes temp file on non-2xx", func(t *testing.T) {
		t.Parallel()

		// arrange
		server := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		dir := t.TempDir()
		destPath := filepath.Join(dir, "out.bin")

		// act
		err := downloadFile(t.Context(), server.URL, destPath, server.Client())

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
		_, statErr := os.Stat(destPath)
		assert.True(t, os.IsNotExist(statErr))
		_, statErr = os.Stat(destPath + ".tmp")
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("with mock client, writes payload from mocked response", func(t *testing.T) {
		t.Parallel()

		// arrange
		client := NewMockHTTPDoer(t)
		client.EXPECT().Do(mock.Anything).Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("mocked-payload")),
		}, nil)

		dir := t.TempDir()
		destPath := filepath.Join(dir, "out.bin")

		// act
		err := downloadFile(t.Context(), "https://example.com/db", destPath, client)

		// assert
		require.NoError(t, err)
		data, err := os.ReadFile(destPath)
		require.NoError(t, err)
		assert.Equal(t, "mocked-payload", string(data))
	})

	t.Run("with mock client, propagates non-2xx status", func(t *testing.T) {
		t.Parallel()

		// arrange
		client := NewMockHTTPDoer(t)
		client.EXPECT().Do(mock.Anything).Return(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil)

		dir := t.TempDir()
		destPath := filepath.Join(dir, "out.bin")

		// act
		err := downloadFile(t.Context(), "https://example.com/db", destPath, client)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
		_, statErr := os.Stat(destPath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("with mock client, propagates transport error", func(t *testing.T) {
		t.Parallel()

		// arrange
		netErr := errors.New("connection refused")
		client := NewMockHTTPDoer(t)
		client.EXPECT().Do(mock.Anything).Return(nil, netErr)

		dir := t.TempDir()
		destPath := filepath.Join(dir, "out.bin")

		// act
		err := downloadFile(t.Context(), "https://example.com/db", destPath, client)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, netErr)
		_, statErr := os.Stat(destPath)
		assert.True(t, os.IsNotExist(statErr))
		_, statErr = os.Stat(destPath + ".tmp")
		assert.True(t, os.IsNotExist(statErr))
	})
}
