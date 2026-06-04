package vpnurl

import (
	"fmt"
	"net/url"
	"strconv"
)

// TrojanOutbound представляет Trojan outbound конфигурацию sing-box.
type TrojanOutbound struct {
	OutboundType string           `json:"type"`
	OutboundTag  string           `json:"tag"`
	Server       string           `json:"server"`
	ServerPort   int              `json:"server_port"`
	Password     string           `json:"password"`
	Network      string           `json:"network,omitempty"`
	TLS          *TLSConfig       `json:"tls,omitempty"`
	Transport    *TransportConfig `json:"transport,omitempty"`
}

// ToOutbound возвращает outbound конфигурацию.
func (o *TrojanOutbound) ToOutbound() any {
	return o
}

// Tag возвращает тег outbound.
func (o *TrojanOutbound) Tag() string {
	return o.OutboundTag
}

// Type возвращает тип outbound.
func (o *TrojanOutbound) Type() string {
	return o.OutboundType
}

// TrojanParser парсит Trojan URL.
type TrojanParser struct{}

// Parse парсит Trojan URL в Trojan outbound конфигурацию.
func (p *TrojanParser) Parse(vpnURL string) (SingBoxOutbound, error) {
	u, err := url.Parse(vpnURL)
	if err != nil {
		return nil, fmt.Errorf("parse trojan url: %w", err)
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("parse trojan port: %w", err)
	}

	query := u.Query()

	outbound := &TrojanOutbound{
		OutboundType: "trojan",
		Server:       u.Hostname(),
		ServerPort:   port,
		Password:     u.User.Username(),
	}

	// Настройка TLS.
	security := query.Get("security")
	if security == "tls" || security == "" {
		outbound.TLS = &TLSConfig{
			Enabled:    true,
			ServerName: query.Get("sni"),
		}
	}

	// Network (tcp/udp).
	if network := query.Get("network"); network != "" {
		outbound.Network = network
	}

	// Транспорт.
	transportType := query.Get("type")
	if transportType != "" && transportType != "tcp" {
		outbound.Transport = &TransportConfig{
			Type: transportType,
			Path: query.Get("path"),
		}
		if host := query.Get("host"); host != "" {
			outbound.Transport.Host = host
		}
	}

	return outbound, nil
}
