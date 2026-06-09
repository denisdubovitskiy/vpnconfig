package subscription

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// defaultTimeout — таймаут HTTP-запроса по умолчанию.
	defaultTimeout = 10 * time.Second
)

// Doer выполняет HTTP-запросы.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client получает список ссылок из base64-encoded подписки.
type Client struct {
	// http — HTTP-клиент для выполнения запросов.
	http Doer
}

// NewClient создаёт новый клиент для base64-encoded подписок.
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
// Ответ декодируется из base64, каждая строка — отдельная ссылка.
func (c *Client) FetchLinks(ctx context.Context, url string) ([]string, error) {
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
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	decoder := base64.NewDecoder(base64.StdEncoding, resp.Body)
	if _, err := buf.ReadFrom(decoder); err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	var links []string
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			links = append(links, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan response: %w", err)
	}

	return links, nil
}
