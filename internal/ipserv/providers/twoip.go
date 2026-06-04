package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// TwoIPBaseURL — базовый URL сервиса api.2ip.me.
const TwoIPBaseURL = "https://api.2ip.me"

// TwoIPClient — клиент для сервиса api.2ip.me.
//
// Используется поле country (английское название), а не country_rus,
// чтобы совпадать с названиями стран в config.yaml.
// Параметр ip передаётся в query string, потому что в URL-пути сервис
// его не принимает.
//
// Пустое поле ip для автоопределения не поддерживается.
type TwoIPClient struct {
	http    Doer
	baseURL string
}

// NewTwoIPClient создаёт новый клиент для api.2ip.me.
func NewTwoIPClient(httpClient Doer) *TwoIPClient {
	return &TwoIPClient{
		http:    httpClient,
		baseURL: TwoIPBaseURL,
	}
}

// CountryByIP выполняет запрос к api.2ip.me для определения страны по IP.
func (c *TwoIPClient) CountryByIP(ctx context.Context, ip string) (*ipserv.Location, error) {
	url := c.baseURL + "/geo.json"
	if ip != "" {
		url = fmt.Sprintf("%s?ip=%s", url, ip)
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
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if body.Country == "" {
		return nil, fmt.Errorf("api error: empty country in response")
	}

	return &ipserv.Location{
		Status:  "success",
		Country: body.Country,
		Query:   ip,
	}, nil
}
