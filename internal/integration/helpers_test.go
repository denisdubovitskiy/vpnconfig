package integration_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Mock HTTP server — обрабатывает 6 geo-провайдеров и 2 типа подписок.
// =============================================================================

// providerHit фиксирует факт обращения к провайдеру.
type providerHit struct {
	IP     string
	Time   time.Time
	Status int
}

// mockHTTPServer инкапсулирует тестовый HTTP-сервер с обработчиками для
// всех 6 geo-провайдеров и двух типов подписок. Сервер ведёт журнал
// обращений для assertion'ов в тестах.
//
// Провайдеры различаются по заголовку X-Original-Host, который
// проставляет rewriteTransport ДО перезаписи URL.
type mockHTTPServer struct {
	t      *testing.T
	server *httptest.Server
	hitsMu sync.Mutex
	hits   map[string][]providerHit
	geoMap map[string]string
}

// newMockHTTPServer создаёт сервер и возвращает его базовый URL.
func newMockHTTPServer(t *testing.T) *mockHTTPServer {
	t.Helper()

	m := &mockHTTPServer{
		t:      t,
		hits:   make(map[string][]providerHit),
		geoMap: make(map[string]string),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/sub/happ", m.handleHappSub)
	mux.HandleFunc("/sub/plain", m.handlePlainSub)
	mux.HandleFunc("/", m.handleGeo) // fallback — все geo-провайдеры

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)

	return m
}

// URL возвращает базовый URL сервера.
func (m *mockHTTPServer) URL() string {
	return m.server.URL
}

// setGeo привязывает IP к стране для всех 6 провайдеров.
func (m *mockHTTPServer) setGeo(ip, country string) {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	m.geoMap[ip] = country
}

// setSubscriptionLinks задаёт список VPN-ссылок, которые вернут
// /sub/happ и /sub/plain.
func (m *mockHTTPServer) setSubscriptionLinks(links []string) {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	m.geoMap["__sub__"] = strings.Join(links, "\n")
}

// setProviderStatus задаёт HTTP-статус ответа для конкретного провайдера.
// Используется для симуляции сбоев: status 500/429 имитирует ошибку.
func (m *mockHTTPServer) setProviderStatus(provider string, status int) {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	m.geoMap["__status__:"+provider] = fmt.Sprintf("%d", status)
}

// hitCount возвращает количество обращений к указанному провайдеру.
func (m *mockHTTPServer) hitCount(provider string) int {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	return len(m.hits[provider])
}

func (m *mockHTTPServer) recordHit(provider, ip string, status int) {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	m.hits[provider] = append(m.hits[provider], providerHit{
		IP: ip, Time: time.Now(), Status: status,
	})
}

func (m *mockHTTPServer) providerStatus(provider string) int {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	if s, ok := m.geoMap["__status__:"+provider]; ok {
		var status int
		_, _ = fmt.Sscanf(s, "%d", &status)
		if status != 0 {
			return status
		}
	}
	return http.StatusOK
}

func (m *mockHTTPServer) countryFor(ip string) string {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	if c, ok := m.geoMap[ip]; ok {
		return c
	}
	return "Unknown"
}

func (m *mockHTTPServer) snapshotSubLinks() string {
	m.hitsMu.Lock()
	defer m.hitsMu.Unlock()
	return m.geoMap["__sub__"]
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

// --- Geo handler — единый диспетчер по X-Original-Host ---

// handleGeo обрабатывает все запросы к geo-провайдерам. Провайдер
// определяется по заголовку X-Original-Host, IP извлекается из path
// или query в зависимости от схемы запроса этого провайдера.
func (m *mockHTTPServer) handleGeo(w http.ResponseWriter, r *http.Request) {
	origHost := r.Header.Get("X-Original-Host")
	if origHost == "" {
		http.Error(w, "missing X-Original-Host header", http.StatusBadRequest)
		return
	}

	provider, ip, ok := extractFromRequest(origHost, r)
	if !ok {
		http.Error(w, "unrecognized path for "+origHost, http.StatusBadRequest)
		return
	}

	m.recordHit(provider, ip, m.providerStatus(provider))
	if m.providerStatus(provider) != http.StatusOK {
		http.Error(w, "provider error", m.providerStatus(provider))
		return
	}

	country := m.countryFor(ip)
	switch provider {
	case "ipapi_co":
		writeJSON(w, map[string]any{
			"ip":      ip,
			"country": country,
		})
	case "ip_api_com":
		writeJSON(w, map[string]any{
			"status":  "success",
			"query":   ip,
			"country": country,
		})
	case "ipwho_is":
		writeJSON(w, map[string]any{
			"ip":      ip,
			"success": true,
			"country": country,
		})
	case "api_2ip_me":
		writeJSON(w, map[string]any{
			"ip":      ip,
			"country": country,
		})
	case "api_ip_sb":
		writeJSON(w, map[string]any{
			"ip":      ip,
			"country": country,
		})
	case "freegeoip_app":
		writeJSON(w, map[string]any{
			"ip":           ip,
			"country_name": country,
		})
	default:
		http.Error(w, "unknown provider: "+provider, http.StatusInternalServerError)
	}
}

// extractFromRequest по production-origin и запросу возвращает имя
// провайдера и IP. Использует точные правила URL каждого провайдера.
func extractFromRequest(origHost string, r *http.Request) (provider, ip string, ok bool) {
	switch origHost {
	case "https://ipapi.co":
		// /<ip>/json/
		ip = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/json/")
		if ip == "" || strings.Contains(ip, "/") {
			return "", "", false
		}
		return "ipapi_co", ip, true
	case "http://ip-api.com":
		// /json/<ip>
		ip = strings.TrimPrefix(r.URL.Path, "/json/")
		if ip == "" || strings.Contains(ip, "/") {
			return "", "", false
		}
		return "ip_api_com", ip, true
	case "https://ipwho.is":
		// /<ip>
		ip = strings.TrimPrefix(r.URL.Path, "/")
		if ip == "" || strings.Contains(ip, "/") {
			return "", "", false
		}
		return "ipwho_is", ip, true
	case "https://api.2ip.me":
		// /geo.json?ip=<ip>
		ip = r.URL.Query().Get("ip")
		if ip == "" {
			return "", "", false
		}
		return "api_2ip_me", ip, true
	case "https://api.ip.sb":
		// /geoip/<ip>
		ip = strings.TrimPrefix(r.URL.Path, "/geoip/")
		if ip == "" || strings.Contains(ip, "/") {
			return "", "", false
		}
		return "api_ip_sb", ip, true
	case "https://freegeoip.app":
		// /json/<ip>
		ip = strings.TrimPrefix(r.URL.Path, "/json/")
		if ip == "" || strings.Contains(ip, "/") {
			return "", "", false
		}
		return "freegeoip_app", ip, true
	}
	return "", "", false
}

// --- Subscription handlers ---

func (m *mockHTTPServer) handleHappSub(w http.ResponseWriter, r *http.Request) {
	m.recordHit("sub_happ", "", http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, base64Std(m.snapshotSubLinks()))
}

func (m *mockHTTPServer) handlePlainSub(w http.ResponseWriter, r *http.Request) {
	m.recordHit("sub_plain", "", http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, m.snapshotSubLinks())
}

// =============================================================================
// rewriteTransport — перенаправляет production URL geo-провайдеров на mock.
// =============================================================================

// rewriteTransport перенаправляет HTTP-запросы к production-хостам
// провайдеров на локальный mock-сервер. Перед перезаписью проставляет
// заголовок X-Original-Host — mock-сервер использует его для
// диспетчеризации по провайдеру.
type rewriteTransport struct {
	base     http.RoundTripper
	rewrites rewriteMap
}

type rewriteMap map[string]string

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	prodBase := req.URL.Scheme + "://" + req.URL.Host

	// Запоминаем оригинальный host для mock-сервера.
	req.Header.Set("X-Original-Host", prodBase)

	if mockBase, ok := t.rewrites[prodBase]; ok {
		mockU, err := url.Parse(mockBase)
		if err == nil {
			req.URL.Scheme = mockU.Scheme
			req.URL.Host = mockU.Host
			req.Host = mockU.Host
		}
	}
	return t.base.RoundTrip(req)
}

// newRewritingClient создаёт http.Client с transport, перенаправляющим
// запросы к production-хостам провайдеров на mock-сервер.
func newRewritingClient(t *testing.T, mock *mockHTTPServer) *http.Client {
	t.Helper()
	mockBase := mock.URL()
	rewrites := rewriteMap{
		"https://ipapi.co":      mockBase,
		"http://ip-api.com":     mockBase,
		"https://ipwho.is":      mockBase,
		"https://api.2ip.me":    mockBase,
		"https://api.ip.sb":     mockBase,
		"https://freegeoip.app": mockBase,
	}
	return &http.Client{
		Transport: &rewriteTransport{
			base:     http.DefaultTransport,
			rewrites: rewrites,
		},
		Timeout: 10 * time.Second,
	}
}

// =============================================================================
// Mock DNS server — минимальный UDP-сервер для A-записей.
// =============================================================================

// dnsServer — минимальный DNS-сервер, отвечающий на A-запросы.
// Поддерживает только самый простой сценарий: один домен → один IP.
// Реализует RFC 1035 в минимальном объёме (без compression, EDNS).
type dnsServer struct {
	addr   string
	domain string
	ip     net.IP
	conn   *net.UDPConn
}

// newDNSServer запускает mock DNS-сервер на 127.0.0.1:0. Возвращает
// адрес в формате "127.0.0.1:PORT" для использования в dns_resolvers.
func newDNSServer(t *testing.T, domain string, ip net.IP) *dnsServer {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("dns: listen: %v", err)
	}
	s := &dnsServer{
		addr:   conn.LocalAddr().String(),
		domain: strings.ToLower(strings.TrimSuffix(domain, ".")),
		ip:     ip.To4(),
		conn:   conn,
	}
	t.Cleanup(func() { _ = conn.Close() })
	go s.serve()
	return s
}

func (s *dnsServer) serve() {
	buf := make([]byte, 512)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if isClosedErr(err) {
				return
			}
			continue
		}
		resp := s.handleQuery(buf[:n])
		if resp != nil {
			_, _ = s.conn.WriteToUDP(resp, clientAddr)
		}
	}
}

func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "closed")
}

func (s *dnsServer) handleQuery(query []byte) []byte {
	if len(query) < 12 {
		return nil
	}
	name, offset, ok := readName(query, 12)
	if !ok {
		return nil
	}
	if offset+4 > len(query) {
		return nil
	}
	qtype := uint16(query[offset])<<8 | uint16(query[offset+1])
	qclass := uint16(query[offset+2])<<8 | uint16(query[offset+3])

	if qtype != 1 /* A */ || qclass != 1 /* IN */ {
		return buildResponse(query, 0)
	}
	if !strings.EqualFold(name, s.domain) {
		return buildResponse(query, 3) // NXDOMAIN
	}
	return buildResponse(query, 0, &dnsAnswer{name: name, ip: s.ip})
}

type dnsAnswer struct {
	name string
	ip   net.IP
}

func readName(query []byte, offset int) (string, int, bool) {
	var labels []string
	for offset < len(query) {
		length := int(query[offset])
		if length == 0 {
			offset++
			return strings.Join(labels, "."), offset, true
		}
		if length&0xC0 != 0 {
			return "", 0, false
		}
		offset++
		if offset+length > len(query) {
			return "", 0, false
		}
		labels = append(labels, string(query[offset:offset+length]))
		offset += length
	}
	return "", 0, false
}

func buildResponse(query []byte, rcode int, answers ...*dnsAnswer) []byte {
	header := make([]byte, 12)
	copy(header, query[:2])
	flags := uint16(0x8180)
	flags |= uint16(rcode & 0x0F)
	header[2] = byte(flags >> 8)
	header[3] = byte(flags & 0xFF)
	header[4] = 0
	header[5] = 1
	header[6] = 0
	header[7] = byte(len(answers))
	header[8] = 0
	header[9] = 0
	header[10] = 0
	header[11] = 0

	resp := append([]byte{}, header...)
	resp = append(resp, query[12:]...)

	for _, a := range answers {
		resp = appendName(resp, a.name)
		resp = append(resp, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
		ip4 := a.ip.To4()
		if ip4 == nil {
			ip4 = net.IPv4zero
		}
		resp = append(resp, ip4...)
	}
	return resp
}

func appendName(buf []byte, name string) []byte {
	if name == "" {
		return append(buf, 0)
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return append(buf, 0)
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	return append(buf, 0)
}
