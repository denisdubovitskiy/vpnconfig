// checker — автономный инструмент для проверкиconnectivity VPN-ссылок.
//
// Запуск:
//
//	go run ./cmd/checker/main.go -config checker_config.yaml
//	go run ./cmd/checker/main.go -config checker_config.yaml -vless "vless://uuid@host:443?..."
//	go run ./cmd/checker/main.go -links links.txt
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"os"
	"strings"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/checker"
	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
	"github.com/denisdubovitskiy/vpnconfig/internal/portgen"
	"github.com/denisdubovitskiy/vpnconfig/internal/profile/subscription"
	"github.com/denisdubovitskiy/vpnconfig/internal/runner"
	"github.com/denisdubovitskiy/vpnconfig/internal/vpnurl"
	"gopkg.in/yaml.v3"
)

// ANSI-цвета для дружелюбного вывода.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// checkerConfig описывает конфигурацию для проверки VPN-ссылок.
type checkerConfig struct {
	// SingBoxPath — путь к утилите sing-box.
	SingBoxPath string `yaml:"sing_box_path,omitempty"`
	// Timeout — максимальное время проверки одной ссылки.
	Timeout string `yaml:"timeout,omitempty"`
	// TestURLs — URL для проверкиconnectivity через прокси.
	TestURLs []string `yaml:"test_urls,omitempty"`
	// Links — список VPN-ссылок для проверки (inline).
	Links []string `yaml:"links,omitempty"`
	// SubscriptionURLs — список URL подписок для получения ссылок.
	SubscriptionURLs []string `yaml:"subscription_urls,omitempty"`
	// SubscriptionURL — один URL подписки (обратная совместимость).
	SubscriptionURL string `yaml:"subscription_url,omitempty"`
	// LinksFile — путь к файлу со ссылками (по одной на строку).
	LinksFile string `yaml:"links_file,omitempty"`
}

// allSubscriptionURLs возвращает объединённый список всех URL подписок.
func (c *checkerConfig) allSubscriptionURLs() []string {
	var urls []string
	urls = append(urls, c.SubscriptionURLs...)
	if c.SubscriptionURL != "" {
		urls = append(urls, c.SubscriptionURL)
	}
	return urls
}

func main() {
	configPath := flag.String("config", "", "путь к конфигурационному файлу YAML")
	vlessLink := flag.String("vless", "", "одна VLESS-ссылка для проверки (быстрый режим)")
	linksFile := flag.String("links", "", "путь к файлу со ссылками (по одной на строку)")
	flag.Parse()

	// Тихий логгер — в CLI-режиме логи не нужны, только вывод пользователя.
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	ctx := logger.IntoContext(context.Background(), log)

	var links []string

	// Способ 1: передана ссылка через флаг -vless
	if *vlessLink != "" {
		links = append(links, *vlessLink)
	}

	// Способ 2: передан файл через флаг -links
	if *linksFile != "" {
		fileLinks, err := loadLinksFromFile(*linksFile)
		if err != nil {
			printError("Ошибка чтения файла ссылок: %v", err)
			os.Exit(1)
		}
		links = append(links, fileLinks...)
	}

	// Способ 3: загрузка из конфига
	if *configPath != "" {
		cfg, err := loadCheckerConfig(*configPath)
		if err != nil {
			printError("Ошибка загрузки конфигурации: %v", err)
			os.Exit(1)
		}

		// Inline ссылки из конфига
		links = append(links, cfg.Links...)

		// Файл со ссылками из конфига
		if cfg.LinksFile != "" {
			fileLinks, err := loadLinksFromFile(cfg.LinksFile)
			if err != nil {
				printError("Ошибка чтения файла ссылок: %v", err)
				os.Exit(1)
			}
			links = append(links, fileLinks...)
		}

		for _, subURL := range cfg.allSubscriptionURLs() {
			subLinks, err := fetchSubscriptionLinks(ctx, subURL)
			if err != nil {
				printError("Ошибка загрузки подписки %s: %v", subURL, err)
				os.Exit(1)
			}
			links = append(links, subLinks...)
		}

		// Применяем настройки из конфига к checker
		applyConfigDefaults(cfg)
	}

	if len(links) == 0 {
		printError("Не указаны ссылки для проверки. Используйте один из способов:")
		fmt.Fprintf(os.Stderr, "  %s-vless \"vless://...\"%s — одна ссылка\n", colorCyan, colorReset)
		fmt.Fprintf(os.Stderr, "  %s-links links.txt%s — файл со ссылками\n", colorCyan, colorReset)
		fmt.Fprintf(os.Stderr, "  %s-config checker_config.yaml%s — конфигурационный файл\n", colorCyan, colorReset)
		os.Exit(1)
	}

	// Фильтруем поддерживаемые протоколы
	supportedLinks := filterSupported(links)
	if len(supportedLinks) == 0 {
		printError("Не найдено ни одной поддерживаемой ссылки (vless://, trojan://, ss://)")
		os.Exit(1)
	}

	runChecker(ctx, supportedLinks)
}

// applyConfigDefaults применяет настройки из конфига к глобальным переменным.
var (
	checkerTimeout  = 10 * time.Second
	checkerSingBox  = "sing-box"
	checkerTestURLs = []string{"https://www.gstatic.com/generate_204"}
)

func applyConfigDefaults(cfg *checkerConfig) {
	if cfg.SingBoxPath != "" {
		checkerSingBox = cfg.SingBoxPath
	}
	if cfg.Timeout != "" {
		d, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			printWarning("Не удалось распарсить timeout %q, используется 10s: %v", cfg.Timeout, err)
		} else {
			checkerTimeout = d
		}
	}
	if len(cfg.TestURLs) > 0 {
		checkerTestURLs = cfg.TestURLs
	}
}

// runChecker выполняет проверку списка VLESS-ссылок.
func runChecker(ctx context.Context, links []string) {
	printBanner()

	parser := vpnurl.NewParser()
	sbRunner := runner.NewProcessRunner(checkerSingBox)
	checkerCfg := checker.Config{
		Enabled:     true,
		SingBoxPath: checkerSingBox,
		Timeout:     checkerTimeout,
		URLs:        checkerTestURLs,
	}
	chk := checker.NewChecker(checkerCfg, parser, sbRunner,
		checker.WithPortGenerator(portgen.New()),
	)

	var passed, failed int
	total := len(links)

	for i, link := range links {
		num := fmt.Sprintf("%d/%d", i+1, total)
		short := truncateURL(link)

		fmt.Fprintf(os.Stderr, "%s[%s]%s Проверяю %s%s%s... ",
			colorDim, num, colorReset,
			colorCyan, short, colorReset)

		linkCtx, cancel := context.WithTimeout(ctx, checkerTimeout)
		err := chk.CheckLink(linkCtx, link)
		cancel()

		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "%s✗ ОШИБКА%s %s(%s)%s\n",
				colorRed, colorReset,
				colorDim, truncateError(err), colorReset)
		} else {
			passed++
			fmt.Fprintf(os.Stderr, "%s✓ OK%s\n", colorGreen, colorReset)
		}
	}

	printSummary(total, passed, failed)
}

// --- Вывод ---

func printBanner() {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s╔══════════════════════════════════════════╗%s\n", colorCyan, colorReset)
	fmt.Fprintf(os.Stderr, "%s║%s   🔍 VPN Link Checker                   %s║%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Fprintf(os.Stderr, "%s╚══════════════════════════════════════════╝%s\n", colorCyan, colorReset)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  %ssing-box:%s  %s\n", colorDim, colorReset, checkerSingBox)
	fmt.Fprintf(os.Stderr, "  %stimeout:%s   %s\n", colorDim, colorReset, checkerTimeout)
	if len(checkerTestURLs) > 0 {
		fmt.Fprintf(os.Stderr, "  %stest url:%s %s\n", colorDim, colorReset, checkerTestURLs[0])
	}
	fmt.Fprintln(os.Stderr)
}

func printSummary(total, passed, failed int) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s──────────────────────────────────────────%s\n", colorDim, colorReset)
	fmt.Fprintf(os.Stderr, "  %sИтого:%s %d ссылок\n", colorBold, colorReset, total)

	if passed > 0 {
		fmt.Fprintf(os.Stderr, "  %s✓ Прошли:%s  %s%d%s\n", colorGreen, colorReset, colorGreen, passed, colorReset)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "  %s✗ Ошибки:%s  %s%d%s\n", colorRed, colorReset, colorRed, failed, colorReset)
	}

	fmt.Fprintln(os.Stderr)

	if failed == 0 {
		fmt.Fprintf(os.Stderr, "  %s🎉 Все ссылки работоспособны!%s\n", colorGreen, colorReset)
	} else {
		fmt.Fprintf(os.Stderr, "  %s⚠️  Есть нерабочие ссылки. Проверьте настройки серверов.%s\n", colorYellow, colorReset)
	}
	fmt.Fprintln(os.Stderr)
}

func printError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\n%s❌ %s%s\n\n", colorRed, msg, colorReset)
}

func printWarning(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s⚠️  %s%s\n", colorYellow, msg, colorReset)
}

// --- Утилиты ---

func truncateURL(vpnURL string) string {
	u, err := parseURLSimple(vpnURL)
	if err != nil {
		if len(vpnURL) > 60 {
			return vpnURL[:57] + "..."
		}
		return vpnURL
	}
	result := u.Scheme + "://" + u.Host
	if len(result) > 60 {
		return result[:57] + "..."
	}
	return result
}

func truncateError(err error) string {
	msg := err.Error()
	// Берём только первую строку ошибки
	if idx := strings.IndexByte(msg, '\n'); idx > 0 {
		msg = msg[:idx]
	}
	if len(msg) > 50 {
		return msg[:47] + "..."
	}
	return msg
}

type simpleURL struct {
	Scheme string
	Host   string
}

func parseURLSimple(raw string) (*simpleURL, error) {
	idx := strings.Index(raw, "://")
	if idx < 0 {
		return nil, fmt.Errorf("no scheme")
	}
	scheme := raw[:idx]
	rest := raw[idx+3:]

	// Убираем путь и query
	slashIdx := strings.Index(rest, "/")
	host := rest
	if slashIdx >= 0 {
		host = rest[:slashIdx]
	}

	return &simpleURL{Scheme: scheme, Host: host}, nil
}

// --- Загрузка данных ---

func loadCheckerConfig(path string) (*checkerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg checkerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

func loadLinksFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var links []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			links = append(links, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}

	return links, nil
}

func fetchSubscriptionLinks(ctx context.Context, url string) ([]string, error) {
	httpClient := &stdhttp.Client{Timeout: 30 * time.Second}
	client := subscription.NewClient(httpClient)
	return client.FetchLinks(ctx, url)
}

func filterSupported(links []string) []string {
	supported := []string{"vless://", "trojan://", "ss://"}
	var result []string
	for _, link := range links {
		for _, prefix := range supported {
			if strings.HasPrefix(link, prefix) {
				result = append(result, link)
				break
			}
		}
	}
	return result
}
