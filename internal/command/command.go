package command

import (
	"context"
	"os/exec"
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
