package command

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Executor выполняет внешние команды.
type Executor interface {
	Exec(ctx context.Context, name string, arg ...string) ([]byte, error)
}

// DefaultExecutor реализует Executor через exec.CommandContext.
type DefaultExecutor struct{}

// NewDefaultExecutor создаёт новый DefaultExecutor.
func NewDefaultExecutor() *DefaultExecutor {
	return &DefaultExecutor{}
}

// Exec выполняет команду и возвращает объединённый вывод stdout+stderr.
func (d *DefaultExecutor) Exec(ctx context.Context, name string, arg ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, arg...).CombinedOutput()
}

// Command управляет жизненным циклом внешнего процесса.
type Command interface {
	// Start запускает процесс.
	Start(ctx context.Context) error
	// Kill принудительно завершает процесс (SIGKILL).
	Kill() error
	// Signal отправляет процессу указанный сигнал.
	Signal(sig os.Signal) error
	// Wait блокируется до завершения процесса и возвращает его exit code.
	Wait() error
	// Stderr возвращает содержимое stderr после завершения процесса.
	Stderr() string
}

// Option опциональный параметр для конструктора Command.
type Option interface {
	apply(*cmd)
}

// cmd реализует Command через os/exec.
type cmd struct {
	name    string
	args    []string
	timeout time.Duration
	stderr  bytes.Buffer
	process *exec.Cmd
}

// New создаёт новый Command для запуска внешнего процесса.
func New(name string, args []string, opts ...Option) Command {
	c := &cmd{
		name: name,
		args: args,
	}
	for _, opt := range opts {
		opt.apply(c)
	}
	return c
}

// WithTimeout устанавливает timeout для ожидания готовности процесса.
func WithTimeout(d time.Duration) Option {
	return timeoutOption{d: d}
}

type timeoutOption struct{ d time.Duration }

func (o timeoutOption) apply(c *cmd) {
	c.timeout = o.d
}

// Start запускает процесс и блокируется до готовности.
// Если timeout не задан — процесс запускается без ожидания готовности.
func (c *cmd) Start(ctx context.Context) error {
	c.process = exec.CommandContext(ctx, c.name, c.args...)
	c.process.Stderr = &c.stderr

	if err := c.process.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	return nil
}

// Kill принудительно завершает процесс.
func (c *cmd) Kill() error {
	if c.process == nil || c.process.Process == nil {
		return nil
	}

	return c.process.Process.Kill()
}

// Signal отправляет процессу указанный сигнал.
func (c *cmd) Signal(sig os.Signal) error {
	if c.process == nil || c.process.Process == nil {
		return nil
	}

	return c.process.Process.Signal(sig)
}

// Wait блокируется до завершения процесса.
func (c *cmd) Wait() error {
	if c.process == nil {
		return nil
	}

	return c.process.Wait()
}

// Stderr возвращает содержимое stderr.
func (c *cmd) Stderr() string {
	return c.stderr.String()
}
