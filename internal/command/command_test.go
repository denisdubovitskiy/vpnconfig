package command

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check: *DefaultExecutor реализует Executor.
var _ Executor = (*DefaultExecutor)(nil)

func findShell(t *testing.T) string {
	t.Helper()
	for _, shell := range []string{"bash", "sh"} {
		if _, err := exec.LookPath(shell); err == nil {
			return shell
		}
	}
	t.Skip("neither bash nor sh is available on this system")
	return ""
}

func TestDefaultExecutor_Exec(t *testing.T) {
	t.Parallel()

	// Проверяем, что stdout-вывод команды возвращается без искажений.
	t.Run("captures stdout", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()

		// act
		output, err := executor.Exec(t.Context(), "printf", "%s", "hello world")

		// assert
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(output))
	})

	// Проверяем, что stderr попадает в combined output.
	t.Run("captures stderr", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()
		shell := findShell(t)

		// act
		output, err := executor.Exec(t.Context(), shell, "-c", "printf stderr-only >&2")

		// assert
		require.NoError(t, err)
		assert.Equal(t, "stderr-only", string(output))
	})

	// Проверяем, что stdout и stderr объединяются.
	// Порядок фрагментов на разных ОС может различаться из-за особенностей
	// runtime, поэтому проверяем только наличие обоих фрагментов.
	t.Run("combines stdout and stderr", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()
		shell := findShell(t)

		// act
		output, err := executor.Exec(
			t.Context(),
			shell,
			"-c",
			"printf out-part; printf err-part >&2",
		)

		// assert
		require.NoError(t, err)
		assert.Contains(t, string(output), "out-part")
		assert.Contains(t, string(output), "err-part")
	})

	// Проверяем, что успешная команда без вывода возвращает пустой результат.
	t.Run("returns empty output for silent success", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()

		// act
		output, err := executor.Exec(t.Context(), "true")

		// assert
		require.NoError(t, err)
		assert.Empty(t, output)
	})

	// Проверяем, что variadic-аргументы передаются в команду в указанном порядке.
	t.Run("passes variadic args in order", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()

		// act
		output, err := executor.Exec(
			t.Context(),
			"printf",
			"%s-%s-%s-%s",
			"alpha",
			"beta",
			"gamma",
			"delta",
		)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "alpha-beta-gamma-delta", string(output))
	})

	// Проверяем, что ненулевой exit code приводит к ошибке и сохранению вывода.
	t.Run("returns error on non-zero exit", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()
		shell := findShell(t)

		// act
		output, err := executor.Exec(
			t.Context(),
			shell,
			"-c",
			"printf diagnostic; exit 7",
		)

		// assert
		require.Error(t, err)
		assert.Equal(t, "diagnostic", string(output))
		assert.Contains(t, err.Error(), "exit status 7")
	})

	// Проверяем, что ошибка содержит вывод, даже если exit code ненулевой
	// и вывод пустой.
	t.Run("returns error on non-zero exit with no output", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()

		// act
		_, err := executor.Exec(t.Context(), "false")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exit status 1")
	})

	// Проверяем, что несуществующая команда возвращает ошибку без паники.
	t.Run("returns error for missing binary", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewDefaultExecutor()

		// act
		_, err := executor.Exec(
			t.Context(),
			"vpnconfig-nonexistent-binary-xyz-12345",
		)

		// assert
		require.Error(t, err)
	})

	// Проверяем, что заранее отменённый контекст прерывает выполнение команды.
	t.Run("cancelled context returns error", func(t *testing.T) {
		t.Parallel()

		// arrange
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		executor := NewDefaultExecutor()
		shell := findShell(t)

		// act
		_, err := executor.Exec(ctx, shell, "-c", "sleep 30")

		// assert
		require.Error(t, err)
	})

	// Проверяем, что истёкший по timeout контекст прерывает выполнение команды.
	t.Run("context timeout returns error", func(t *testing.T) {
		t.Parallel()

		// arrange
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()
		executor := NewDefaultExecutor()
		shell := findShell(t)

		// act
		_, err := executor.Exec(ctx, shell, "-c", "sleep 30")

		// assert
		require.Error(t, err)
	})
}
