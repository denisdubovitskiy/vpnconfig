package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// IPSBBaseURL — базовый URL сервиса api.ip.sb.
const IPSBBaseURL = "https://api.ip.sb"

// IPSBClient — клиент для сервиса api.ip.sb.
//
// Сервис не возвращает явный статус успеха/ошибки — при успехе поле country
// непустое, при ошибке ответ может быть пустым или содержать message.
type IPSBClient struct {
	http    Doer
	baseURL string
}

// NewIPSBClient создаёт новый клиент для api.ip.sb.
func NewIPSBClient(httpClient Doer) *IPSBClient {
	return &IPSBClient{
		http:    httpClient,
		baseURL: IPSBBaseURL,
	}
}

// CountryByIP выполняет запрос к api.ip.sb для определения страны по IP.
// Если ip пустая строка — определяется страна текущего соединения.
func (c *IPSBClient) CountryByIP(ctx context.Context, ip string) (*ipserv.Location, error) {
	url := c.baseURL + "/geoip/"
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
	//nolint:errcheck // defer Close — стандартная идиома.
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var body struct {
		Country string `json:"country"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if body.Country == "" {
		return nil, fmt.Errorf("api error: %s", body.Message)
	}

	return &ipserv.Location{
		Status:  "success",
		Country: body.Country,
		Query:   ip,
	}, nil
}
