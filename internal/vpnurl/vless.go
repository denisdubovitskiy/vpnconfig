package vpnurl

import (
	"fmt"
	"net/url"
	"strconv"
)

// VLESSOutbound представляет VLESS outbound конфигурацию sing-box.
type VLESSOutbound struct {
	OutboundType string           `json:"type"`
	OutboundTag  string           `json:"tag"`
	Server       string           `json:"server"`
	ServerPort   int              `json:"server_port"`
	UUID         string           `json:"uuid"`
	Flow         string           `json:"flow,omitempty"`
	Network      string           `json:"network,omitempty"`
	TLS          *TLSConfig       `json:"tls,omitempty"`
	Transport    *TransportConfig `json:"transport,omitempty"`
}

// ToOutbound возвращает outbound конфигурацию.
func (o *VLESSOutbound) ToOutbound() any {
	return o
}

// Tag возвращает тег outbound.
func (o *VLESSOutbound) Tag() string {
	return o.OutboundTag
}

// Type возвращает тип outbound.
func (o *VLESSOutbound) Type() string {
	return o.OutboundType
}

// VlessParser парсит VLESS URL.
type VlessParser struct{}

// Parse парсит VLESS URL в VLESS outbound конфигурацию.
func (p *VlessParser) Parse(vpnURL string) (SingBoxOutbound, error) {
	u, err := url.Parse(vpnURL)
	if err != nil {
		return nil, fmt.Errorf("parse vless url: %w", err)
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("parse vless port: %w", err)
	}

	query := u.Query()

	outbound := &VLESSOutbound{
		OutboundType: "vless",
		Server:       u.Hostname(),
		ServerPort:   port,
		UUID:         u.User.Username(),
		Flow:         query.Get("flow"),
	}

	// Настройка TLS.
	security := query.Get("security")
	if security == "tls" || security == "reality" {
		outbound.TLS = &TLSConfig{
			Enabled:    true,
			ServerName: query.Get("sni"),
		}

		// uTLS fingerprint.
		fp := query.Get("fp")
		if fp != "" {
			outbound.TLS.UTLS = &UTLSConfig{
				Enabled:     true,
				Fingerprint: fp,
			}
		}

		// Reality.
		if security == "reality" {
			outbound.TLS.Reality = &RealityConfig{
				Enabled:   true,
				PublicKey: query.Get("pbk"),
				ShortID:   query.Get("sid"),
			}
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
		}
		if transportType == "grpc" {
			outbound.Transport.ServiceName = query.Get("serviceName")
		}
		if host := query.Get("host"); host != "" {
			outbound.Transport.Host = host
		}
	}

	return outbound, nil
}
