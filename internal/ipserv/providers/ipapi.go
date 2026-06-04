package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// IPAPIBaseURL — базовый URL сервиса ipapi.co.
const IPAPIBaseURL = "https://ipapi.co"

// IPAPIClient — клиент для сервиса ipapi.co.
//
// Используется JSON-эндпоинт /json/, потому что текстовый /country_name/
// отдаёт Cloudflare challenge. При ошибке сервис возвращает
// {"error": true, "reason": "..."} с HTTP 200.
type IPAPIClient struct {
	http    Doer
	baseURL string
}

// NewIPAPIClient создаёт новый клиент для ipapi.co.
func NewIPAPIClient(httpClient Doer) *IPAPIClient {
	return &IPAPIClient{
		http:    httpClient,
		baseURL: IPAPIBaseURL,
	}
}

// CountryByIP выполняет запрос к ipapi.co для определения страны по IP.
// Если ip пустая строка — определяется страна текущего соединения.
func (c *IPAPIClient) CountryByIP(ctx context.Context, ip string) (*ipserv.Location, error) {
	url := c.baseURL + "/json/"
	if ip != "" {
		url = fmt.Sprintf("%s/%s/json/", c.baseURL, ip)
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
		Error   bool   `json:"error"`
		Reason  string `json:"reason"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if body.Error {
		return nil, fmt.Errorf("api error: %s", body.Reason)
	}

	return &ipserv.Location{
		Status:  "success",
		Country: body.Country,
		Query:   ip,
	}, nil
}
