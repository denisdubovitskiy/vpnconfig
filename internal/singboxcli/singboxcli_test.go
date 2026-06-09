package singboxcli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Compile-time checks: обе реализации удовлетворяют ConfigValidator.
var (
	_ ConfigValidator = (*CLIChecker)(nil)
	_ ConfigValidator = (*NullChecker)(nil)
)

const (
	// testConfigPath — путь к конфигу, который передаётся в CheckConfig.
	testConfigPath = "/etc/sing-box/singbox.json"
	// testConfigFlag — флаг, который CLIChecker передаёт в CLI первым аргументом.
	testConfigFlag = "--config"
	// testCheckSubcommand — финальный аргумент, вызывающий проверку конфига.
	testCheckSubcommand = "check"
	// testDefaultCLI — бинарь по умолчанию из PATH, если cliPath не задан.
	testDefaultCLI = "sing-box"
)

func TestCLIChecker_CheckConfig(t *testing.T) {
	t.Parallel()

	// Проверяем успешный сценарий, когда в конструктор передан явный путь к CLI.
	t.Run("success with custom cli path", func(t *testing.T) {
		t.Parallel()

		// arrange
		const customCLIPath = "/opt/sing-box/bin/sing-box"
		executor := NewMockCommandExecutor(t)
		executor.
			EXPECT().
			Exec(
				mock.Anything,
				customCLIPath,
				[]string{testConfigFlag, testConfigPath, testCheckSubcommand},
			).
			Return([]byte(""), nil)

		checker := NewCLIChecker(customCLIPath, executor)

		// act
		err := checker.CheckConfig(t.Context(), testConfigPath)

		// assert
		require.NoError(t, err)
	})

	// Проверяем, что пустой cliPath подменяется на "sing-box" из PATH.
	t.Run("uses default sing-box when cli path is empty", func(t *testing.T) {
		t.Parallel()

		// arrange
		executor := NewMockCommandExecutor(t)
		executor.
			EXPECT().
			Exec(
				mock.Anything,
				testDefaultCLI,
				[]string{testConfigFlag, testConfigPath, testCheckSubcommand},
			).
			Return([]byte(""), nil)

		checker := NewCLIChecker("", executor)

		// act
		err := checker.CheckConfig(t.Context(), testConfigPath)

		// assert
		require.NoError(t, err)
	})

	// Проверяем, что ошибка выполнения команды оборачивается и содержит
	// и оригинальную ошибку, и вывод команды.
	t.Run("wraps error and includes command output on failure", func(t *testing.T) {
		t.Parallel()

		// arrange
		const diagOutput = "Error: invalid configuration at line 42"
		execErr := errors.New("exit status 1")
		executor := NewMockCommandExecutor(t)
		executor.
			EXPECT().
			Exec(
				mock.Anything,
				testDefaultCLI,
				[]string{testConfigFlag, testConfigPath, testCheckSubcommand},
			).
			Return([]byte(diagOutput), execErr)

		checker := NewCLIChecker("", executor)

		// act
		err := checker.CheckConfig(t.Context(), testConfigPath)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, execErr, "original exec error must be wrapped")
		assert.Contains(t, err.Error(), "sing-box check failed")
		assert.Contains(t, err.Error(), diagOutput)
	})

	// Проверяем, что пустой вывод команды не приводит к панике и формирует
	// корректное сообщение об ошибке.
	t.Run("handles empty error output without panic", func(t *testing.T) {
		t.Parallel()

		// arrange
		execErr := errors.New("exit status 1")
		executor := NewMockCommandExecutor(t)
		executor.
			EXPECT().
			Exec(
				mock.Anything,
				testDefaultCLI,
				[]string{testConfigFlag, testConfigPath, testCheckSubcommand},
			).
			Return([]byte{}, execErr)

		checker := NewCLIChecker("", executor)

		// act
		err := checker.CheckConfig(t.Context(), testConfigPath)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, execErr)
		assert.Contains(t, err.Error(), "sing-box check failed")
	})

	// Проверяем, что trailing whitespace в выводе команды вырезается
	// при формировании сообщения об ошибке.
	t.Run("trims trailing whitespace from output in error message", func(t *testing.T) {
		t.Parallel()

		// arrange
		const cleanOutput = "Error: bad config"
		const paddedOutput = cleanOutput + "\n\n  \t"
		executor := NewMockCommandExecutor(t)
		executor.
			EXPECT().
			Exec(
				mock.Anything,
				testDefaultCLI,
				[]string{testConfigFlag, testConfigPath, testCheckSubcommand},
			).
			Return([]byte(paddedOutput), errors.New("exit status 1"))

		checker := NewCLIChecker("", executor)

		// act
		err := checker.CheckConfig(t.Context(), testConfigPath)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "output: "+cleanOutput)
		assert.NotContains(t, err.Error(), "\n\n  \t", "whitespace must be trimmed")
	})

	// Проверяем, что переданный контекст пробрасывается в executor без изменений
	// (smoke-тест на корректность передачи ctx).
	t.Run("propagates caller context to executor", func(t *testing.T) {
		t.Parallel()

		// arrange
		type ctxKey struct{}
		ctx := context.WithValue(t.Context(), ctxKey{}, "vpnconfig-marker")
		executor := NewMockCommandExecutor(t)
		executor.
			EXPECT().
			Exec(
				mock.MatchedBy(func(c context.Context) bool {
					return c.Value(ctxKey{}) == "vpnconfig-marker"
				}),
				testDefaultCLI,
				[]string{testConfigFlag, testConfigPath, testCheckSubcommand},
			).
			Return([]byte(""), nil)

		checker := NewCLIChecker("", executor)

		// act
		err := checker.CheckConfig(ctx, testConfigPath)

		// assert
		require.NoError(t, err)
	})
}

func TestNullChecker_CheckConfig(t *testing.T) {
	t.Parallel()

	// Проверяем, что NullChecker всегда возвращает nil, не обращаясь к
	// пути и контексту.
	t.Run("returns nil for any path", func(t *testing.T) {
		t.Parallel()

		// arrange
		checker := NewNullChecker()

		// act
		err := checker.CheckConfig(t.Context(), "/any/sing-box.json")

		// assert
		require.NoError(t, err)
	})

	// Проверяем, что NullChecker не чувствителен к состоянию контекста —
	// даже отменённый контекст не приводит к ошибке.
	t.Run("returns nil even with cancelled context", func(t *testing.T) {
		t.Parallel()

		// arrange
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		checker := NewNullChecker()

		// act
		err := checker.CheckConfig(ctx, "/any/sing-box.json")

		// assert
		require.NoError(t, err)
	})

	// Проверяем, что повторные вызовы NullChecker остаются стабильными.
	t.Run("returns nil on repeated invocations", func(t *testing.T) {
		t.Parallel()

		// arrange
		checker := NewNullChecker()

		// act + assert
		for range 3 {
			require.NoError(t, checker.CheckConfig(t.Context(), "/any/sing-box.json"))
		}
	})
}
