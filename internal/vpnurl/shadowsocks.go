package vpnurl

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ShadowsocksOutbound представляет Shadowsocks outbound конфигурацию sing-box.
type ShadowsocksOutbound struct {
	OutboundType string `json:"type"`
	OutboundTag  string `json:"tag"`
	Server       string `json:"server"`
	ServerPort   int    `json:"server_port"`
	Method       string `json:"method"`
	Password     string `json:"password"`
}

// ToOutbound возвращает outbound конфигурацию.
func (o *ShadowsocksOutbound) ToOutbound() any {
	return o
}

// Tag возвращает тег outbound.
func (o *ShadowsocksOutbound) Tag() string {
	return o.OutboundTag
}

// Type возвращает тип outbound.
func (o *ShadowsocksOutbound) Type() string {
	return o.OutboundType
}

// ShadowsocksParser парсит Shadowsocks URL.
type ShadowsocksParser struct{}

// Parse парсит Shadowsocks URL в Shadowsocks outbound конфигурацию.
// Поддерживает форматы:
//
//	ss://method:password@host:port
//	ss://base64(method:password)@host:port
func (p *ShadowsocksParser) Parse(vpnURL string) (SingBoxOutbound, error) {
	u, err := url.Parse(vpnURL)
	if err != nil {
		return nil, fmt.Errorf("parse shadowsocks url: %w", err)
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("parse shadowsocks port: %w", err)
	}

	method := u.User.Username()
	password, hasPassword := u.User.Password()

	// Если password пустой, возможно method — это base64 строка "method:password".
	if !hasPassword || password == "" {
		decoded, decodeErr := base64.StdEncoding.DecodeString(method)
		if decodeErr == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				method = parts[0]
				password = parts[1]
			}
		}
	} else {
		// Пробуем декодировать password из base64.
		decoded, decodeErr := base64.StdEncoding.DecodeString(password)
		if decodeErr == nil {
			password = string(decoded)
		}
	}

	outbound := &ShadowsocksOutbound{
		OutboundType: "shadowsocks",
		Server:       u.Hostname(),
		ServerPort:   port,
		Method:       method,
		Password:     password,
	}

	return outbound, nil
}
