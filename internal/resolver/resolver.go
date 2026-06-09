// Package resolver собирает DNS-резолвер из списка URL с поддержкой
// DNS over HTTPS (https://), DNS over TLS (tls://) и plain DNS (IP:port).
//
// Поддерживаемые схемы:
//
//   - "https://host/path" — DNS over HTTPS (DoH). Путь обязателен.
//   - "tls://host[:port]" — DNS over TLS (DoT).
//   - "host[:port]" или IP — plain DNS. Порт по умолчанию 53.
//
// При резолве домена Resolver пробует первый IPResolver в цепочке, при
// неудаче переходит к следующему (fallback chain). Это повышает надёжность:
// если один из резолверов недоступен, запрос автоматически уходит к другому.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/ncruces/go-dns"
)

// IPResolver резолвит доменное имя в список IP-адресов.
// Сигнатура совпадает с *net.Resolver.LookupIP, поэтому и *net.Resolver,
// и сгенерированные mockery-моки могут использоваться в цепочке.
type IPResolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

const (
	// defaultPlainDNSPort — UDP/TCP-порт для plain DNS.
	defaultPlainDNSPort = "53"
	// plainDNSDialTimeout — таймаут установки TCP/UDP-соединения с plain DNS.
	plainDNSDialTimeout = 5 * time.Second
)

// Kind резолвера, полученный из URL.
const (
	kindDoH   = "doh"
	kindDoT   = "dot"
	kindPlain = "plain"
)

// Resolver резолвит доменные имена через цепочку IPResolver'ов.
// При неуспехе первого резолвера автоматически переходит к следующему.
type Resolver struct {
	// chain — упорядоченный список резолверов: первый — primary,
	// остальные — fallback в порядке очереди.
	chain []IPResolver
}

// Option конфигурирует Resolver при создании через New.
type Option func(*Resolver)

// WithResolver добавляет IPResolver в цепочку. Можно вызывать несколько
// раз — каждый вызов добавляет резолвер в конец цепочки. Порядок
// вызовов определяет порядок fallback.
//
// В production используется с резолверами, собранными из URL через
// OptionsFromURLs. В тестах — с MockIPResolver из mockery.
func WithResolver(r IPResolver) Option {
	return func(rsv *Resolver) {
		rsv.chain = append(rsv.chain, r)
	}
}

// New создаёт Resolver из набора опций. Без опций возвращает пустой
// Resolver (LookupIP вернёт ошибку "no dns resolvers configured").
//
// Возвращаемый тип — *Resolver, а не error: при использовании WithResolver
// ошибок быть не может; валидация URL выполняется в OptionsFromURLs.
func New(opts ...Option) *Resolver {
	r := &Resolver{}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// OptionsFromURLs парсит список URL-адресов DNS-резолверов и возвращает
// набор опций для New. Каждый URL валидируется сразу; при ошибке
// возвращается (nil, error).
//
// Порядок URL определяет порядок резолверов в цепочке: первый — primary.
func OptionsFromURLs(urls []string) ([]Option, error) {
	opts := make([]Option, 0, len(urls))
	for i, raw := range urls {
		parsed, err := parseURL(raw)
		if err != nil {
			return nil, fmt.Errorf("resolver url #%d (%q): %w", i+1, raw, err)
		}

		r, err := buildResolver(parsed)
		if err != nil {
			return nil, fmt.Errorf("resolver url #%d (%q): %w", i+1, raw, err)
		}
		opts = append(opts, WithResolver(r))
	}
	return opts, nil
}

// LookupIP резолвит доменное имя через цепочку резолверов. Пробует
// каждый резолвер по порядку; первый успешный результат (непустой
// слайс IP) возвращается. Если все резолверы вернули ошибку или пустой
// результат — возвращается последняя ошибка (или обобщённая ошибка,
// если ошибок не было).
//
// Сигнатура совпадает с *net.Resolver.LookupIP, поэтому *Resolver
// можно использовать везде, где ожидается *net.Resolver.
func (r *Resolver) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	if r == nil || len(r.chain) == 0 {
		return nil, errors.New("no dns resolvers configured")
	}

	var lastErr error
	for _, resolver := range r.chain {
		ips, err := resolver.LookupIP(ctx, network, host)
		if err != nil {
			lastErr = err
			continue
		}
		if len(ips) == 0 {
			lastErr = fmt.Errorf("resolver returned no addresses for %s", host)
			continue
		}
		return ips, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all dns resolvers failed: %w", lastErr)
	}
	return nil, fmt.Errorf("all dns resolvers returned no addresses for %s", host)
}

// parsedURL — результат разбора одной строки конфигурации резолвера.
type parsedURL struct {
	// kind — тип резолвера: "doh", "dot", "plain".
	kind string
	// host — адрес резолвера в формате host или host:port.
	host string
	// path — путь DoH-эндпоинта (только для kind="doh").
	path string
}

// parseURL разбирает строку конфигурации резолвера. Допустимые схемы
// см. в package doc; для строк без схемы считает формат plain DNS.
func parseURL(raw string) (*parsedURL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("empty url")
	}

	if !strings.Contains(trimmed, "://") {
		return &parsedURL{
			kind: kindPlain,
			host: ensurePort(trimmed, defaultPlainDNSPort),
		}, nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "https":
		if u.Path == "" || u.Path == "/" {
			return nil, errors.New("DoH url path is required (e.g. https://dns.google/dns-query)")
		}
		return &parsedURL{
			kind: kindDoH,
			host: u.Host,
			path: u.Path,
		}, nil
	case "tls":
		return &parsedURL{
			kind: kindDoT,
			host: u.Host,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported scheme %q (available: https, tls, plain)", u.Scheme)
	}
}

// ensurePort добавляет порт к адресу, если он не указан.
func ensurePort(host, defaultPort string) string {
	if host == "" {
		return host
	}
	if strings.HasPrefix(host, "[") {
		if strings.HasSuffix(host, "]") {
			return host + ":" + defaultPort
		}
		if _, _, err := net.SplitHostPort(host); err == nil {
			return host
		}
		return host + "]:" + defaultPort
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return host + ":" + defaultPort
}

// buildResolver создаёт IPResolver на основе распарсенного URL.
func buildResolver(p *parsedURL) (IPResolver, error) {
	switch p.kind {
	case kindDoH:
		return newDoHResolver(p.host, p.path)
	case kindDoT:
		return newDoTResolver(p.host)
	case kindPlain:
		return newPlainResolver(p.host), nil
	}
	return nil, fmt.Errorf("unknown resolver kind %q", p.kind)
}

// newDoHResolver создаёт DoH-резолвер. uri собирается из host и path:
// "https://" + host + path.
func newDoHResolver(host, path string) (IPResolver, error) {
	uri := "https://" + host + path
	r, err := dns.NewDoHResolver(uri)
	if err != nil {
		return nil, fmt.Errorf("create DoH resolver: %w", err)
	}
	return &netResolverAdapter{r: r}, nil
}

// newDoTResolver создаёт DoT-резолвер для адреса host (например,
// "dns.google" или "1.1.1.1:853"). Порт по умолчанию подставляет
// сама библиотека ncruces/go-dns.
func newDoTResolver(host string) (IPResolver, error) {
	r, err := dns.NewDoTResolver(host)
	if err != nil {
		return nil, fmt.Errorf("create DoT resolver: %w", err)
	}
	return &netResolverAdapter{r: r}, nil
}

// newPlainResolver создаёт plain DNS-резолвер, который форвардит
// все запросы на указанный address (host:port).
func newPlainResolver(address string) IPResolver {
	return &plainResolver{address: address}
}

// netResolverAdapter оборачивает *net.Resolver от ncruces/go-dns
// в локальный IPResolver (нужно для тестирования через mock'и других
// реализаций и для единообразия API).
type netResolverAdapter struct {
	r *net.Resolver
}

func (a *netResolverAdapter) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return a.r.LookupIP(ctx, network, host)
}

const (
	dnsFlagRD         uint16 = 0x0100
	dnsQTypeA         uint16 = 1
	dnsQClassIN       uint16 = 1
	dnsTypeA          uint16 = 1
	dnsRDataIPv4             = 4
	dnsQuestionTail          = 4 // QTYPE(2) + QCLASS(2) после QNAME
	dnsAnswerClassTTL        = 8 // CLASS(2) + TTL(4) после TYPE
)

// plainResolver отправляет DNS-запросы через UDP напрямую, минуя net.Resolver.
// Это нужно потому, что net.Resolver.Dial игнорируется на macOS/iOS и
// используется системный резолвер в обход кастомного Dial. Реализация —
// минимальный DNS-клиент по RFC 1035 (только A-записи), совместимый
// с любым стандартным DNS-сервером.
type plainResolver struct {
	address string
}

func (p *plainResolver) LookupIP(ctx context.Context, _, host string) ([]net.IP, error) {
	query := buildDNSQuery(host)

	d := net.Dialer{Timeout: plainDNSDialTimeout}
	conn, err := d.DialContext(ctx, "udp", p.address)
	if err != nil {
		return nil, fmt.Errorf("dial dns server: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(plainDNSDialTimeout)); err != nil {
		return nil, fmt.Errorf("set dns deadline: %w", err)
	}

	if _, err := conn.Write(query); err != nil {
		return nil, fmt.Errorf("write dns query: %w", err)
	}

	resp := make([]byte, 512)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("read dns response: %w", err)
	}

	return parseDNSAResponse(resp[:n])
}

func buildDNSQuery(host string) []byte {
	buf := make([]byte, 0, 12+len(host)+dnsQuestionTail)
	buf = append(buf,
		0x12, 0x34,
		byte(dnsFlagRD>>8), byte(dnsFlagRD&0xFF),
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	)
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			continue
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf,
		0x00,
		byte(dnsQTypeA>>8), byte(dnsQTypeA&0xFF),
		byte(dnsQClassIN>>8), byte(dnsQClassIN&0xFF),
	)
	return buf
}

func parseDNSAResponse(resp []byte) ([]net.IP, error) {
	if len(resp) < 12 {
		return nil, fmt.Errorf("dns response too short: %d bytes", len(resp))
	}
	rcode := resp[3] & 0x0F
	if rcode != 0 {
		return nil, fmt.Errorf("dns server returned rcode %d", rcode)
	}
	offset := 12

	qdcount := int(uint16(resp[4])<<8 | uint16(resp[5]))
	for i := 0; i < qdcount; i++ {
		newOffset, err := skipDNSName(resp, offset)
		if err != nil {
			return nil, err
		}
		offset = newOffset + dnsQuestionTail
	}

	ancount := int(uint16(resp[6])<<8 | uint16(resp[7]))
	var ips []net.IP
	for i := 0; i < ancount; i++ {
		newOffset, err := skipDNSName(resp, offset)
		if err != nil {
			return nil, err
		}
		offset = newOffset
		if offset+10 > len(resp) {
			return nil, fmt.Errorf("dns response truncated at answer header")
		}
		qtype := uint16(resp[offset])<<8 | uint16(resp[offset+1])
		offset += dnsAnswerClassTTL
		rdlength := int(uint16(resp[offset])<<8 | uint16(resp[offset+1]))
		offset += 2
		if offset+rdlength > len(resp) {
			return nil, fmt.Errorf("dns response truncated at rdata")
		}
		if qtype == dnsTypeA && rdlength == dnsRDataIPv4 {
			ips = append(ips, net.IP(resp[offset:offset+rdlength]))
		}
		offset += rdlength
	}
	return ips, nil
}

func skipDNSName(resp []byte, offset int) (int, error) {
	for {
		if offset >= len(resp) {
			return 0, fmt.Errorf("dns name out of bounds")
		}
		length := int(resp[offset])
		offset++
		if length == 0 {
			return offset, nil
		}
		if length&0xC0 != 0 {
			return offset + 1, nil
		}
		offset += length
	}
}
