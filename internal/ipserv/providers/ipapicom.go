package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// IPAPICOMBaseURL — базовый URL сервиса ip-api.com.
const IPAPICOMBaseURL = "http://ip-api.com/json"

// IPAPICOMClient — клиент для сервиса ip-api.com.
//
// Бесплатный тариф работает только по HTTP. Ответ приходит в JSON с
// полем status: "success" при удаче и status: "fail" + message при ошибке.
type IPAPICOMClient struct {
	http    Doer
	baseURL string
}

// NewIPAPICOMClient создаёт новый клиент для ip-api.com.
func NewIPAPICOMClient(httpClient Doer) *IPAPICOMClient {
	return &IPAPICOMClient{
		http:    httpClient,
		baseURL: IPAPICOMBaseURL,
	}
}

// CountryByIP выполняет запрос к ip-api.com для определения страны по IP.
// Если ip пустая строка — определяется страна текущего соединения.
func (c *IPAPICOMClient) CountryByIP(ctx context.Context, ip string) (*ipserv.Location, error) {
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

	var loc ipserv.Location
	if err := json.NewDecoder(resp.Body).Decode(&loc); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if loc.Status != "success" {
		return nil, fmt.Errorf("api error: %s", loc.Message)
	}

	return &loc, nil
}
