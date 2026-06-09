package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
	"github.com/denisdubovitskiy/vpnconfig/internal/portgen"
	"github.com/denisdubovitskiy/vpnconfig/internal/runner"
	"github.com/denisdubovitskiy/vpnconfig/internal/singbox"
	"github.com/denisdubovitskiy/vpnconfig/internal/vpnurl"
)

type Config struct {
	Enabled      bool
	SingBoxPath  string
	TmpDirectory string
	Timeout      time.Duration
	URLs         []string
}

// Константы по умолчанию для checker.
const (
	defaultCheckTimeout = 10 * time.Second

	socksInboundTag   = "checker-socks-in"
	directOutboundTag = "direct"

	defaultTestURL = "https://www.gstatic.com/generate_204"
)

// SingBoxRunner определяет контракт для запуска sing-box внутри checker.
// Дублирует runner.SingBoxRunner, чтобы mockery мог генерировать моки
// в том же пакете (_test.go-файлы не экспортируются).
type SingBoxRunner interface {
	Start(ctx context.Context, configPath string, port int) error
	Stop(ctx context.Context) error
}

// compile-time проверка совместимости с runner.SingBoxRunner.
var _ SingBoxRunner = runner.SingBoxRunner(nil)
var _ runner.SingBoxRunner = SingBoxRunner(nil)

// Checker проверяет работоспособность VPN-ссылок через локальный sing-box.
type Checker interface {
	// CheckLink проверяет одну VPN-ссылку, запуская локальный sing-box.
	CheckLink(ctx context.Context, vlessLink string) error
}

// HttpDoer выполняет HTTP-запросы. Используется вместо *http.Client,
// чтобы тесты могли подменять транспорт без запуска реального SOCKS5.
type HttpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type PortGenerator interface {
	RandomPort(ctx context.Context) (port int, err error)
}

// CheckerOption конфигурирует linkChecker.
type CheckerOption interface {
	apply(*linkChecker)
}

type checkerOptionFunc func(*linkChecker)

func (f checkerOptionFunc) apply(c *linkChecker) { f(c) }

// WithHTTPDoer переопределяет HTTP-клиент для проверки URL (для тестов).
func WithHTTPDoer(d HttpDoer) CheckerOption {
	return checkerOptionFunc(func(c *linkChecker) { c.doer = d })
}

// WithPortGenerator переопределяет генератор портов (для тестов).
func WithPortGenerator(g PortGenerator) CheckerOption {
	return checkerOptionFunc(func(c *linkChecker) { c.portGen = g })
}

// NewChecker создаёт новый Checker.
func NewChecker(cfg Config, parser *vpnurl.Parser, sbRunner SingBoxRunner, opts ...CheckerOption) Checker {
	c := &linkChecker{
		cfg:     cfg,
		parser:  parser,
		runner:  sbRunner,
		doer:    defaultHTTPClient(),
		portGen: portgen.New(),
	}
	for _, opt := range opts {
		opt.apply(c)
	}
	return c
}

// linkChecker реализует Checker.
type linkChecker struct {
	cfg     Config
	parser  *vpnurl.Parser
	runner  SingBoxRunner
	doer    HttpDoer
	portGen PortGenerator
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Second,
			}).DialContext,
		},
		Timeout: 5 * time.Second,
	}
}

// CheckLink проверяет одну VPN-ссылку.
func (c *linkChecker) CheckLink(ctx context.Context, vlessLink string) (err error) {
	log := logger.FromContext(ctx)
	truncated := truncateURL(vlessLink)
	log.Info("checking link", "url", truncated)
	defer func() {
		if err != nil {
			log.Warn("link check failed", "url", truncated, "error", err.Error())
		} else {
			log.Info("link check passed", "url", truncated)
		}
	}()

	timeout := c.cfg.Timeout
	if timeout == 0 {
		timeout = defaultCheckTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	outbound, err := c.parser.Parse(vlessLink)
	if err != nil {
		return fmt.Errorf("parse vpn url: %w", err)
	}

	sbOutbound, err := singbox.ConvertFromSingBoxOutbound(outbound.ToOutbound())
	if err != nil {
		return fmt.Errorf("convert outbound: %w", err)
	}

	port, err := c.portGen.RandomPort(ctx)
	if err != nil {
		return fmt.Errorf("get random port: %w", err)
	}
	log.Debug("allocated port for check", "port", port)

	tmpDir := c.cfg.TmpDirectory
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}

	configPath := filepath.Join(tmpDir, fmt.Sprintf("singbox-check-%d.json", time.Now().UnixNano()))
	data, err := buildConfig(sbOutbound, port)
	if err != nil {
		return fmt.Errorf("build sing-box config: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	defer func() { _ = os.Remove(configPath) }()

	if err := c.runner.Start(ctx, configPath, port); err != nil {
		return fmt.Errorf("start sing-box: %w", err)
	}
	log.Debug("sing-box started for check", "port", port)
	defer func() { _ = c.runner.Stop(ctx) }()

	testURLs := c.cfg.URLs
	if len(testURLs) == 0 {
		testURLs = []string{defaultTestURL}
	}

	for _, testURL := range testURLs {
		log.Debug("testing url through proxy", "test_url", testURL, "port", port)
		if err := c.checkURL(ctx, testURL, port); err != nil {
			return fmt.Errorf("check url %s: %w", testURL, err)
		}
	}

	return nil
}

func buildConfig(outbound singbox.Outbound, port int) ([]byte, error) {
	tag := outbound.Tag()

	cfg := map[string]any{
		"log": map[string]any{
			"level": "error",
		},
		"inbounds": []any{
			map[string]any{
				"type":        "socks",
				"tag":         socksInboundTag,
				"listen":      "127.0.0.1",
				"listen_port": port,
			},
		},
		"outbounds": []any{
			outbound,
			map[string]any{
				"type": "direct",
				"tag":  directOutboundTag,
			},
		},
		"route": map[string]any{
			"rules": []any{},
			"final": tag,
		},
	}

	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return append(data, '\n'), nil
}

func (c *linkChecker) checkURL(ctx context.Context, testURL string, port int) error {
	proxyURL, err := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("parse proxy url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	client := c.injectableClient(proxyURL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// injectableClient возвращает HTTP-клиент с SOCKS5-прокси.
// Для *http.Client создаётся новый Transport с прокси, чтобы не мутировать
// общий клиент; для мока возвращается как есть.
func (c *linkChecker) injectableClient(proxyURL *url.URL) HttpDoer {
	client, ok := c.doer.(*http.Client)
	if !ok {
		return c.doer
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
		}).DialContext,
		Proxy: http.ProxyURL(proxyURL),
	}

	return &http.Client{
		Transport: transport,
		Timeout:   client.Timeout,
	}
}

// truncateURL обрезает URL для логирования, оставляя только схему и хост.
func truncateURL(vpnURL string) string {
	u, err := url.Parse(vpnURL)
	if err != nil {
		return vpnURL
	}
	return u.Scheme + "://" + u.Host
}
