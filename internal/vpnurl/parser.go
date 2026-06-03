package vpnurl

import (
	"fmt"
	"strings"
)

// SchemeParser парсит VPN URL конкретной схемы в outbound-конфигурацию sing-box.
type SchemeParser interface {
	// Parse парсит VPN URL в outbound.
	Parse(vpnURL string) (SingBoxOutbound, error)
}

// SingBoxOutbound представляет outbound-конфигурацию sing-box.
type SingBoxOutbound interface {
	// ToOutbound возвращает структуру outbound в формате sing-box.
	ToOutbound() any
	// Tag возвращает тег outbound.
	Tag() string
	// Type возвращает тип outbound.
	Type() string
}

// Parser парсит VPN URL в outbound-конфигурацию sing-box.
type Parser struct {
	parsers map[string]SchemeParser
}

// NewParser создаёт новый парсер VPN URL с дефолтными парсерами схем.
func NewParser() *Parser {
	return &Parser{
		parsers: map[string]SchemeParser{
			"vless":  &VlessParser{},
			"trojan": &TrojanParser{},
			"ss":     &ShadowsocksParser{},
		},
	}
}

// NewParserWithParsers создаёт парсер с пользовательскими парсерами схем.
// Полезно для тестирования.
func NewParserWithParsers(parsers map[string]SchemeParser) *Parser {
	return &Parser{parsers: parsers}
}

// Parse определяет схему VPN URL и делегирует парсинг соответствующему парсеру.
func (p *Parser) Parse(vpnURL string) (SingBoxOutbound, error) {
	if !strings.Contains(vpnURL, "://") {
		return nil, fmt.Errorf("invalid vpn url: no scheme")
	}

	scheme := strings.Split(vpnURL, "://")[0]

	parser, ok := p.parsers[scheme]
	if !ok {
		return nil, fmt.Errorf("unsupported scheme: %s", scheme)
	}

	return parser.Parse(vpnURL)
}

// TLSConfig представляет TLS конфигурацию.
type TLSConfig struct {
	Enabled    bool           `json:"enabled"`
	ServerName string         `json:"server_name,omitempty"`
	UTLS       *UTLSConfig    `json:"utls,omitempty"`
	Reality    *RealityConfig `json:"reality,omitempty"`
}

// UTLSConfig представляет uTLS конфигурацию.
type UTLSConfig struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// RealityConfig представляет Reality конфигурацию.
type RealityConfig struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
}

// TransportConfig представляет транспортную конфигурацию.
type TransportConfig struct {
	Type        string `json:"type,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	Path        string `json:"path,omitempty"`
}
