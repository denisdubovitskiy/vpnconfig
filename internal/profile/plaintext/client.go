package plaintext

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
)

const (
	// defaultTimeout — таймаут HTTP-запроса по умолчанию.
	defaultTimeout = 10 * time.Second
)

// Doer выполняет HTTP-запросы.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client получает список ссылок из plain text подписки.
type Client struct {
	// http — HTTP-клиент для выполнения запросов.
	http Doer
}

// NewClient создаёт новый клиент для plain text подписок.
func NewClient(httpClient Doer) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: defaultTimeout,
		}
	}

	return &Client{
		http: httpClient,
	}
}

// FetchLinks выполняет запрос и возвращает список ссылок.
// Ответ считывается как plain text, каждая непустая строка — отдельная ссылка.
func (c *Client) FetchLinks(ctx context.Context, url string) ([]string, error) {
	log := logger.FromContext(ctx)
	log.Debug("fetching plaintext links", "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	//nolint:errcheck // стандартная идиома игнорировать ошибку Close body.
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn("plaintext subscription returned non-200", "url", url, "status", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var links []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			links = append(links, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan response: %w", err)
	}

	log.Debug("plaintext response received", "url", url, "status", resp.StatusCode, "links_count", len(links))
	return links, nil
}
