package command

import (
	"context"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// Compile-time check: *cmd реализует Command.
var _ Command = (*cmd)(nil)

func TestCommand_StartAndKill(t *testing.T) {
	t.Parallel()

	// arrange
	shell := findShell(t)
	cmd := New(shell, []string{"-c", "sleep 30"})

	// act
	err := cmd.Start(t.Context())

	// assert
	require.NoError(t, err)
	require.NoError(t, cmd.Kill())
	// Wait возвращает ошибку, потому что процесс был убит
	_ = cmd.Wait()
}

func TestCommand_Signal(t *testing.T) {
	t.Parallel()

	// arrange
	shell := findShell(t)
	cmd := New(shell, []string{"-c", "trap '' TERM; sleep 30"})

	err := cmd.Start(t.Context())
	require.NoError(t, err)

	// act
	err = cmd.Signal(syscall.SIGTERM)

	// assert
	require.NoError(t, err)
	_ = cmd.Wait()
}

func TestCommand_Stderr(t *testing.T) {
	t.Parallel()

	// arrange
	shell := findShell(t)
	cmd := New(shell, []string{"-c", "echo error-output >&2; exit 1"})

	// act
	err := cmd.Start(t.Context())
	require.NoError(t, err)
	waitErr := cmd.Wait()

	// assert
	require.Error(t, waitErr)
	assert.Contains(t, cmd.Stderr(), "error-output")
}

func TestCommand_StartError(t *testing.T) {
	t.Parallel()

	// arrange
	cmd := New("nonexistent-binary-xyz-12345", nil)

	// act
	err := cmd.Start(t.Context())

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start process")
}

func TestCommand_KillNilProcess(t *testing.T) {
	t.Parallel()

	// arrange
	cmd := New("true", nil)

	// act — Kill до Start не должен паниковать
	err := cmd.Kill()

	// assert
	require.NoError(t, err)
}

func TestCommand_SignalNilProcess(t *testing.T) {
	t.Parallel()

	// arrange
	cmd := New("true", nil)

	// act — Signal до Start не должен паниковать
	err := cmd.Signal(syscall.SIGTERM)

	// assert
	require.NoError(t, err)
}

func TestCommand_WaitNilProcess(t *testing.T) {
	t.Parallel()

	// arrange
	cmd := New("true", nil)

	// act — Wait до Start не должен паниковать
	err := cmd.Wait()

	// assert
	require.NoError(t, err)
}

func TestCommand_WithContextCancel(t *testing.T) {
	t.Parallel()

	// arrange
	shell := findShell(t)
	ctx, cancel := context.WithCancel(t.Context())
	cmd := New(shell, []string{"-c", "sleep 30"})

	err := cmd.Start(ctx)
	require.NoError(t, err)

	// act
	cancel()

	// assert
	_ = cmd.Wait() // процесс завершается из-за отмены контекста
}

func TestDefaultExecutor_Exec(t *testing.T) {
	t.Parallel()

	t.Run("successful command", func(t *testing.T) {
		t.Parallel()

		// arrange
		exe := NewDefaultExecutor()

		// act
		out, err := exe.Exec(t.Context(), "echo", "hello-from-executor")

		// assert
		require.NoError(t, err)
		assert.Contains(t, string(out), "hello-from-executor")
	})

	t.Run("command not found", func(t *testing.T) {
		t.Parallel()

		// arrange
		exe := NewDefaultExecutor()

		// act
		_, err := exe.Exec(t.Context(), "nonexistent-binary-xyz-99999")

		// assert
		require.Error(t, err)
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		// arrange
		exe := NewDefaultExecutor()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		shell := findShell(t)

		// act
		_, err := exe.Exec(ctx, shell, "-c", "sleep 30")

		// assert
		require.Error(t, err)
	})
}

func TestCommand_WithTimeout(t *testing.T) {
	t.Parallel()

	// arrange
	shell := findShell(t)
	cmd := New(shell, []string{"-c", "sleep 30"}, WithTimeout(100))

	// act
	err := cmd.Start(t.Context())

	// assert
	require.NoError(t, err)
	require.NoError(t, cmd.Kill())
	_ = cmd.Wait()
}
