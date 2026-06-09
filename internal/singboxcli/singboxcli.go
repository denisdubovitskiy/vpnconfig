package singboxcli

import (
	"context"
	"fmt"
	"strings"

	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
)

// ConfigValidator проверяет валидность конфигурации sing-box.
type ConfigValidator interface {
	// CheckConfig проверяет конфигурацию sing-box по указанному пути.
	// Возвращает nil, если конфиг валиден, или ошибку с описанием проблемы.
	CheckConfig(ctx context.Context, configPath string) error
}

// Executor выполняет внешние команды.
type CommandExecutor interface {
	Exec(ctx context.Context, name string, arg ...string) ([]byte, error)
}

// CLIChecker реализует ConfigValidator через вызов sing-box CLI.
type CLIChecker struct {
	// cliPath — путь к утилите sing-box. Если пустая строка — используется "sing-box" из PATH.
	cliPath string
	// executor — выполняет внешние команды.
	executor CommandExecutor
}

// NewCLIChecker создаёт новый CLIChecker.
// Если cliPath пустая строка, будет использоваться "sing-box" из PATH.
func NewCLIChecker(cliPath string, executor CommandExecutor) *CLIChecker {
	return &CLIChecker{
		cliPath:  cliPath,
		executor: executor,
	}
}

// CheckConfig проверяет конфигурацию sing-box, вызывая команду check.
// Формат команды: sing-box --config <path> check
func (c *CLIChecker) CheckConfig(ctx context.Context, configPath string) (err error) {
	cli := c.cliPath
	if cli == "" {
		cli = "sing-box"
	}

	log := logger.FromContext(ctx)
	log.Debug("validating sing-box config", "path", configPath, "cli", cli)
	defer func() {
		if err != nil {
			log.Warn("sing-box config validation failed", "path", configPath, "error", err.Error())
		} else {
			log.Debug("sing-box config validation passed", "path", configPath)
		}
	}()

	args := []string{"--config", configPath, "check"}

	output, err := c.executor.Exec(ctx, cli, args...)
	if err != nil {
		return fmt.Errorf("sing-box check failed: %w, output: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// NullChecker — заглушка для ConfigValidator, которая всегда возвращает nil.
// Используется, когда проверка конфига не требуется (singbox_cli не настроен).
type NullChecker struct{}

// NewNullChecker создаёт новый NullChecker.
func NewNullChecker() *NullChecker {
	return &NullChecker{}
}

// CheckConfig всегда возвращает nil.
func (n *NullChecker) CheckConfig(_ context.Context, _ string) error {
	return nil
}
