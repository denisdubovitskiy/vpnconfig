package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	// Проверяем создание логгера с записью в файл.
	t.Run("creates log file", func(t *testing.T) {
		t.Parallel()

		// arrange
		logDir := t.TempDir()

		// act
		log, closer, err := New(logDir)

		// assert
		require.NoError(t, err)
		require.NotNil(t, log)
		require.NotNil(t, closer)

		entries, err := os.ReadDir(logDir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.True(t, strings.HasPrefix(entries[0].Name(), "vpnconfig_"))
		assert.True(t, strings.HasSuffix(entries[0].Name(), ".log"))

		require.NoError(t, closer.Close())
	})

	// Проверяем запись сообщения в файл.
	t.Run("writes log message to file", func(t *testing.T) {
		t.Parallel()

		// arrange
		logDir := t.TempDir()

		// act
		log, closer, err := New(logDir)
		require.NoError(t, err)

		log.Info("test message", "key", "value")
		require.NoError(t, closer.Close())

		// assert
		entries, err := os.ReadDir(logDir)
		require.NoError(t, err)
		require.Len(t, entries, 1)

		data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "test message")
		assert.Contains(t, content, "key=value")
	})

	// Проверяем работу с пустой директорией.
	t.Run("empty log dir returns stderr only logger", func(t *testing.T) {
		t.Parallel()

		// act
		log, closer, err := New("")

		// assert
		require.NoError(t, err)
		require.NotNil(t, log)
		require.NotNil(t, closer)
		require.NoError(t, closer.Close())
	})

	// Проверяем создание директории если она не существует.
	t.Run("creates log directory if not exists", func(t *testing.T) {
		t.Parallel()

		// arrange
		parentDir := t.TempDir()
		logDir := filepath.Join(parentDir, "nested", "logs")

		// act
		log, closer, err := New(logDir)

		// assert
		require.NoError(t, err)
		require.NotNil(t, log)
		require.NotNil(t, closer)

		info, err := os.Stat(logDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		require.NoError(t, closer.Close())
	})

	// Проверяем что closer закрывает файл.
	t.Run("closer closes file", func(t *testing.T) {
		t.Parallel()

		// arrange
		logDir := t.TempDir()
		_, closer, err := New(logDir)
		require.NoError(t, err)

		// act
		err = closer.Close()

		// assert
		require.NoError(t, err)
	})
}

func TestIntoContext(t *testing.T) {
	t.Parallel()

	// Проверяем, что nil-логгер возвращает исходный ctx без изменений.
	t.Run("nil log returns ctx unchanged", func(t *testing.T) {
		t.Parallel()

		// arrange
		ctx := t.Context()

		// act
		result := IntoContext(ctx, nil)

		// assert
		assert.Same(t, ctx, result)
	})

	// Проверяем, что не-nil логгер кладётся в ctx и достаётся обратно.
	t.Run("stores log in context", func(t *testing.T) {
		t.Parallel()

		// arrange
		ctx := t.Context()
		log := Silent()

		// act
		wrapped := IntoContext(ctx, log)
		got := FromContext(wrapped)

		// assert
		assert.Same(t, log, got)
	})
}

func TestFromContext(t *testing.T) {
	t.Parallel()

	// Проверяем fallback на slog.Default при отсутствии логгера в ctx.
	t.Run("returns default when no log in ctx", func(t *testing.T) {
		t.Parallel()

		// arrange
		ctx := t.Context()

		// act
		got := FromContext(ctx)

		// assert
		assert.Equal(t, slog.Default(), got)
	})

	// Проверяем, что логгер, положенный через IntoContext, достаётся обратно.
	t.Run("returns log set via IntoContext", func(t *testing.T) {
		t.Parallel()

		// arrange
		log := Silent()
		ctx := IntoContext(t.Context(), log)

		// act
		got := FromContext(ctx)

		// assert
		assert.Same(t, log, got)
	})
}

func TestSilent(t *testing.T) {
	t.Parallel()

	// Проверяем, что Silent возвращает не-nil логгер.
	t.Run("returns non-nil logger", func(t *testing.T) {
		t.Parallel()

		// act
		log := Silent()

		// assert
		require.NotNil(t, log)
	})

	// Проверяем, что вызовы методов Silent-логгера не паникуют.
	t.Run("logger accepts calls without panic", func(t *testing.T) {
		t.Parallel()

		// arrange
		log := Silent()

		// act
		callFn := func() {
			log.Info("test", "key", "value")
			log.Warn("warn")
			log.Error("error")
			log.Debug("debug")
		}

		// assert
		assert.NotPanics(t, callFn)
	})
}
