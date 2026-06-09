package runner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/command"
)

// Константы по умолчанию для запуска sing-box.
const (
	defaultCLIPath         = "sing-box"
	defaultDialTimeout     = 100 * time.Millisecond
	defaultPollDelay       = 50 * time.Millisecond
	defaultStartTimeout    = 5 * time.Second
	defaultStopGracePeriod = 2 * time.Second
)

// Sentinel-ошибки для идентификации сценариев снаружи пакета.
var (
	// ErrAlreadyStarted возвращается, если Start вызван повторно без Stop.
	ErrAlreadyStarted = errors.New("sing-box is already running")
	// ErrNotStarted возвращается из Stop, если процесс не был запущен.
	ErrNotStarted = errors.New("sing-box is not running")
	// ErrStartTimeout возвращается, если sing-box не начал слушать за отведённое время.
	ErrStartTimeout = errors.New("sing-box did not start in time")
)

// SingBoxRunner запускает и останавливает sing-box процесс.
type SingBoxRunner interface {
	// Start запускает sing-box с указанным конфигом на заданном порту
	// и ждёт, пока он начнёт слушать.
	Start(ctx context.Context, configPath string, port int) error
	// Stop останавливает sing-box и ждёт завершения процесса.
	Stop(ctx context.Context) error
}

// ProcessController определяет контракт для управления внешним процессом.
type ProcessController interface {
	Start(ctx context.Context) error
	Kill() error
	Signal(sig os.Signal) error
	Wait() error
	Stderr() string
}

// Conn — локальный контракт для net.Conn, чтобы mockery мог генерировать моки
// в том же пакете.
type Conn interface {
	net.Conn
}

// Dialer определяет контракт для установки TCP-соединений.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// CommandFactory создаёт ProcessController для запуска sing-box.
type CommandFactory func(name string, args []string) ProcessController

// Option опциональный параметр для processRunner.
type Option interface {
	apply(*processRunner)
}

// WithCommandFactory переопределяет фабрику команд (для тестов).
func WithCommandFactory(f CommandFactory) Option {
	return optionFunc(func(r *processRunner) { r.factory = f })
}

// WithDialer переопределяет dialer для проверки порта (для тестов).
func WithDialer(d Dialer) Option {
	return optionFunc(func(r *processRunner) { r.dialer = d })
}

// WithPollDelay переопределяет задержку между попытками подключения.
func WithPollDelay(d time.Duration) Option {
	return optionFunc(func(r *processRunner) { r.pollDelay = d })
}

// WithNowFunc переопределяет функцию получения текущего времени (для тестов).
func WithNowFunc(fn func() time.Time) Option {
	return optionFunc(func(r *processRunner) { r.nowFunc = fn })
}

// WithStartTimeout переопределяет таймаут старта при отсутствии deadline в контексте.
func WithStartTimeout(d time.Duration) Option {
	return optionFunc(func(r *processRunner) { r.startTimeout = d })
}

// WithStopGracePeriod переопределяет время ожидания graceful shutdown (для тестов).
func WithStopGracePeriod(d time.Duration) Option {
	return optionFunc(func(r *processRunner) { r.stopGracePeriod = d })
}

type optionFunc func(*processRunner)

func (f optionFunc) apply(r *processRunner) { f(r) }

// processRunner реализует SingBoxRunner через запуск sing-box как внешнего процесса.
type processRunner struct {
	cliPath         string
	factory         CommandFactory
	dialer          Dialer
	pollDelay       time.Duration
	startTimeout    time.Duration
	stopGracePeriod time.Duration
	nowFunc         func() time.Time

	mu  sync.Mutex
	cmd ProcessController
}

// NewProcessRunner создаёт новый processRunner.
// Если cliPath пустая строка, будет использоваться "sing-box" из PATH.
func NewProcessRunner(cliPath string, opts ...Option) SingBoxRunner {
	r := &processRunner{
		cliPath: cliPath,
		factory: func(name string, args []string) ProcessController {
			return command.New(name, args)
		},
		dialer:          &net.Dialer{Timeout: defaultDialTimeout},
		pollDelay:       defaultPollDelay,
		startTimeout:    defaultStartTimeout,
		stopGracePeriod: defaultStopGracePeriod,
		nowFunc:         time.Now,
	}
	for _, opt := range opts {
		opt.apply(r)
	}
	return r
}

func (r *processRunner) Start(ctx context.Context, configPath string, port int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd != nil {
		return ErrAlreadyStarted
	}

	cli := r.cliPath
	if cli == "" {
		cli = defaultCLIPath
	}

	cmd := r.factory(cli, []string{"run", "-c", configPath})

	if err := cmd.Start(ctx); err != nil {
		return fmt.Errorf("start sing-box: %w", err)
	}

	if err := r.waitForReady(ctx, port); err != nil {
		_ = cmd.Kill()
		_ = cmd.Wait()
		if stderr := cmd.Stderr(); stderr != "" {
			return fmt.Errorf("%w: %s", err, stderr)
		}
		return err
	}

	r.cmd = cmd
	return nil
}

func (r *processRunner) Stop(_ context.Context) error {
	r.mu.Lock()
	cmd := r.cmd
	gracePeriod := r.stopGracePeriod
	r.cmd = nil
	r.mu.Unlock()

	if cmd == nil {
		return nil
	}

	_ = cmd.Signal(os.Interrupt)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(gracePeriod):
		_ = cmd.Kill()
		<-done
		return nil
	}
}

func (r *processRunner) waitForReady(ctx context.Context, port int) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = r.nowFunc().Add(r.startTimeout)
	}

	ticker := time.NewTicker(r.pollDelay)
	defer ticker.Stop()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for r.nowFunc().Before(deadline) {
		conn, err := r.dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ErrStartTimeout
		case <-ticker.C:
		}
	}

	return ErrStartTimeout
}
