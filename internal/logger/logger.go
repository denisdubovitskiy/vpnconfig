// Package logger отвечает за инициализацию структурированного логгера
// и за безопасный обмен логгером между горутинами через context.Context.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type ctxKey struct{}

// IntoContext возвращает копию ctx, в которую помещён log. Последующие
// вызовы FromContext с этим контекстом вернут тот же log. Если log == nil,
// возвращается ctx без изменений — на случай, когда логгер ещё не создан.
func IntoContext(ctx context.Context, log *slog.Logger) context.Context {
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, log)
}

// FromContext достаёт *slog.Logger из ctx. Если логгер не задан — возвращает
// slog.Default() как безопасный fallback: код остаётся работоспособным даже
// при отсутствии явной инициализации.
func FromContext(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && log != nil {
		return log
	}
	return slog.Default()
}

// Silent возвращает *slog.Logger, который игнорирует все сообщения.
// Используется в тестах: logger.IntoContext(t.Context(), logger.Silent())
// превращает контекст в «тихий», и логи не пишутся в stderr при прогоне.
func Silent() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// New создаёт логгер, который пишет одновременно в os.Stderr и в файл.
// Если logDir пустая — возвращает логгер, пишущий только в os.Stderr.
// Файл логов именуется с timestamp: vpnconfig_YYYYMMDD_HHMMSS.log.
func New(logDir string) (*slog.Logger, io.Closer, error) {
	consoleHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	if logDir == "" {
		return slog.New(consoleHandler), noopCloser{}, nil
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}

	filename := fmt.Sprintf("vpnconfig_%s.log", time.Now().Format("20060102_150405"))
	logPath := filepath.Join(logDir, filename)

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	multi := io.MultiWriter(os.Stderr, file)
	multiHandler := slog.NewTextHandler(multi, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	return slog.New(multiHandler), file, nil
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
