package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// IPWhoIsBaseURL — базовый URL сервиса ipwho.is.
const IPWhoIsBaseURL = "https://ipwho.is"

// IPWhoIsClient — клиент для сервиса ipwho.is.
//
// При успехе сервис возвращает success: true, при ошибке — success: false
// и поле message с описанием причины.
type IPWhoIsClient struct {
	http    Doer
	baseURL string
}

// NewIPWhoIsClient создаёт новый клиент для ipwho.is.
func NewIPWhoIsClient(httpClient Doer) *IPWhoIsClient {
	return &IPWhoIsClient{
		http:    httpClient,
		baseURL: IPWhoIsBaseURL,
	}
}

// CountryByIP выполняет запрос к ipwho.is для определения страны по IP.
// Если ip пустая строка — определяется страна текущего соединения.
func (c *IPWhoIsClient) CountryByIP(ctx context.Context, ip string) (*ipserv.Location, error) {
	url := c.baseURL
	if ip != "" {
		url = fmt.Sprintf("%s/%s", c.baseURL, ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	//nolint:errcheck // defer Close — стандартная идиома.
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var body struct {
		Country string `json:"country"`
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !body.Success {
		return nil, fmt.Errorf("api error: %s", body.Message)
	}

	return &ipserv.Location{
		Status:  "success",
		Country: body.Country,
		Query:   ip,
	}, nil
}
