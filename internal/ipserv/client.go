package ipserv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultTimeout — таймаут HTTP-запроса по умолчанию.
	defaultTimeout = 10 * time.Second
	// baseURL — базовый URL API ip-api.com.
	baseURL = "http://ip-api.com/json"
)

// Doer выполняет HTTP-запросы.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Location представляет ответ от API ip-api.com.
// Содержит только поля, необходимые для определения страны.
type Location struct {
	// Status — статус ответа ("success" или "fail").
	Status string `json:"status"`
	// Country — название страны.
	Country string `json:"country"`
	// Query — запрашиваемый IP-адрес.
	Query string `json:"query"`
	// Message — сообщение об ошибке (при status == "fail").
	Message string `json:"message"`
}

// Client — HTTP-клиент для сервиса ip-api.com.
type Client struct {
	// http — HTTP-клиент для выполнения запросов.
	http Doer
	// baseURL — базовый URL API.
	baseURL string
}

// NewClient создаёт новый клиент для ip-api.com.
func NewClient(httpClient Doer) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: defaultTimeout,
		}
	}

	return &Client{
		http:    httpClient,
		baseURL: baseURL,
	}
}

// CountryByIP выполняет запрос к API ip-api.com для определения страны по IP-адресу.
// Если ip пустая строка — определяется страна текущего соединения.
func (c *Client) CountryByIP(ctx context.Context, ip string) (*Location, error) {
	url := c.baseURL
	if ip != "" {
		url = fmt.Sprintf("%s/%s", c.baseURL, ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var loc Location
	if err := json.NewDecoder(resp.Body).Decode(&loc); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if loc.Status != "success" {
		return nil, fmt.Errorf("api error: %s", loc.Message)
	}

	return &loc, nil
}
