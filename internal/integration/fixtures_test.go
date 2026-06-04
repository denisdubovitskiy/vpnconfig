package integration_test

import (
	"encoding/base64"
	"fmt"
)

// =============================================================================
// Тестовые IP-адреса. Используются только в e2e-тестах; сопоставление
// IP -> страна задаётся в mockHTTPServer через setGeo.
// =============================================================================

const (
	// West-европейские IP (RFC 5737 TEST-NET-1).
	ipSweden      = "192.0.2.1"
	ipNetherlands = "192.0.2.2"
	ipUSA         = "192.0.2.3"
	ipLithuania   = "192.0.2.4"

	// Российский IP (RFC 5737 TEST-NET-2).
	ipRussia = "198.51.100.1"

	// IP, возвращаемый mock DNS-сервером для test.example.com.
	ipFromDNS = "203.0.113.1"
)

const (
	countrySweden      = "Sweden"
	countryNetherlands = "Netherlands"
	countryUSA         = "United States"
	countryLithuania   = "Lithuania"
	countryRussia      = "Russia"
	countryDNSResolved = "Sweden" // IP из DNS попадает в MULTI_WEST
)

const testDomain = "test.example.com"

// =============================================================================
// Шаблон VPN-ссылок. Все учётные данные — фиктивные. Структура валидная.
// =============================================================================

// Шаблон для VLESS-ссылки. UUID — нули (детерминированный, безопасен
// для тестов). Параметры — type=tcp, security=reality, fp, sni, sid, pbk.
func vlessURL(uuid, server, name string) string {
	return fmt.Sprintf(
		"vless://%s@%s:443?type=tcp&security=reality&pbk=TestPublicKey&fp=chrome&sni=example.com&sid=ab&flow=xtls-rprx-vision#%s",
		uuid, server, name,
	)
}

// Шаблон для Trojan-ссылки.
func trojanURL(password, server, name string) string {
	return fmt.Sprintf(
		"trojan://%s@%s:443?security=tls&sni=example.com&type=tcp#%s",
		password, server, name,
	)
}

// Шаблон для Shadowsocks-ссылки. credentials — base64(method:password).
func ssURL(method, password, server, name string) string {
	cred := base64.StdEncoding.EncodeToString([]byte(method + ":" + password))
	return fmt.Sprintf("ss://%s@%s:8388#%s", cred, server, name)
}

// vmessJSONURL формирует vmess:// ссылку из JSON-конфига.
// Используется только для проверки, что vmess пропускается.
func vmessJSONURL(server string) string {
	cfg := fmt.Sprintf(`{"v":"2","ps":"vmess-test","add":%q,"port":443,"id":"00000000-0000-0000-0000-000000000000","aid":0,"type":"none"}`, server)
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(cfg))
}

// westVPNLinks возвращает 16 ссылок для MULTI_WEST: 4 IP × 4 варианта
// (vless-reality, vless-tls, trojan-tls, ss-2022). Каждый IP даёт по 4
// ссылки с разными именами.
func westVPNLinks() []string {
	servers := []struct {
		ip      string
		country string
	}{
		{ipSweden, countrySweden},
		{ipNetherlands, countryNetherlands},
		{ipUSA, countryUSA},
		{ipLithuania, countryLithuania},
	}

	uuids := []string{
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000002",
		"00000000-0000-0000-0000-000000000003",
		"00000000-0000-0000-0000-000000000004",
	}

	links := make([]string, 0, 4*len(servers))
	for i, s := range servers {
		links = append(links,
			vlessURL(uuids[i], s.ip, fmt.Sprintf("vless-r-%s-1", s.country)),
			trojanURL("test-pwd-1", s.ip, fmt.Sprintf("trojan-%s-1", s.country)),
			ssURL("aes-256-gcm", "test-pwd-1", s.ip, fmt.Sprintf("ss-gcm-%s-1", s.country)),
			ssURL("2022-blake3-aes-256-gcm", "test-pwd-2", s.ip, fmt.Sprintf("ss-22-%s-1", s.country)),
		)
	}
	return links
}

// ruVPNLinks возвращает 4 ссылки для MULTI_RU: 1 IP × 4 варианта.
func ruVPNLinks() []string {
	uuids := []string{
		"00000000-0000-0000-0000-000000000101",
		"00000000-0000-0000-0000-000000000102",
		"00000000-0000-0000-0000-000000000103",
		"00000000-0000-0000-0000-000000000104",
	}
	return []string{
		vlessURL(uuids[0], ipRussia, "vless-russia-1"),
		trojanURL("test-pwd-ru-1", ipRussia, "trojan-russia-1"),
		ssURL("aes-256-gcm", "test-pwd-ru-1", ipRussia, "ss-gcm-russia-1"),
		ssURL("2022-blake3-aes-256-gcm", "test-pwd-ru-2", ipRussia, "ss-22-russia-1"),
	}
}

// vmessLink — vmess-ссылка, которая должна быть пропущена.
func vmessLink() string {
	return vmessJSONURL(ipRussia)
}

// domainLink — VLESS-ссылка с доменом (для DNS-теста). Домен резолвится
// в ipFromDNS, и эта ссылка попадает в MULTI_WEST.
func domainLink() string {
	uuid := "00000000-0000-0000-0000-000000000200"
	return vlessURL(uuid, testDomain, "vless-dns-test")
}

// fullSubscriptionLinks — набор для основного e2e-теста:
// 16 west + 4 ru + 1 vmess (skipped) = 21 ссылок; 20 пройдут парсинг.
func fullSubscriptionLinks() []string {
	links := westVPNLinks()
	links = append(links, ruVPNLinks()...)
	links = append(links, vmessLink())
	return links
}

// domainSubscriptionLinks — набор для DNS-теста: 1 west + 1 domain-based.
func domainSubscriptionLinks() []string {
	links := westVPNLinks()[:4] // 4 ссылки: первые 4 IP × 1 протокол каждая
	links = append(links, domainLink())
	return links
}
