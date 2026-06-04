package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// FreeGeoIPBaseURL — базовый URL сервиса freegeoip.app.
const FreeGeoIPBaseURL = "https://freegeoip.app"

// FreeGeoIPClient — клиент для сервиса freegeoip.app.
//
// Название страны возвращается в поле country_name (не country).
// При недоступности сервиса отвечает HTTP 403, что и обрабатывается
// общим правилом через status code.
type FreeGeoIPClient struct {
	http    Doer
	baseURL string
}

// NewFreeGeoIPClient создаёт новый клиент для freegeoip.app.
func NewFreeGeoIPClient(httpClient Doer) *FreeGeoIPClient {
	return &FreeGeoIPClient{
		http:    httpClient,
		baseURL: FreeGeoIPBaseURL,
	}
}

// CountryByIP выполняет запрос к freegeoip.app для определения страны по IP.
// Если ip пустая строка — определяется страна текущего соединения.
func (c *FreeGeoIPClient) CountryByIP(ctx context.Context, ip string) (*ipserv.Location, error) {
	url := c.baseURL + "/json/"
	if ip != "" {
		url = fmt.Sprintf("%s%s", url, ip)
	}

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

	var body struct {
		CountryName string `json:"country_name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if body.CountryName == "" {
		return nil, fmt.Errorf("api error: empty country_name in response")
	}

	return &ipserv.Location{
		Status:  "success",
		Country: body.CountryName,
		Query:   ip,
	}, nil
}
