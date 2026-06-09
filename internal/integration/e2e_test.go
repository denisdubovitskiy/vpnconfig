package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv/providers"
	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
	"github.com/denisdubovitskiy/vpnconfig/internal/profile"
	"github.com/denisdubovitskiy/vpnconfig/internal/profile/plaintext"
	"github.com/denisdubovitskiy/vpnconfig/internal/profile/subscription"
	"github.com/denisdubovitskiy/vpnconfig/internal/resolver"
	"github.com/denisdubovitskiy/vpnconfig/internal/singbox"
	"github.com/denisdubovitskiy/vpnconfig/internal/singboxcli"
	"github.com/denisdubovitskiy/vpnconfig/internal/updater"
	"github.com/denisdubovitskiy/vpnconfig/internal/vpnurl"
)

func providersBuild(t *testing.T, name string, client *http.Client) (ipserv.IPLookup, error) {
	t.Helper()
	return providers.NewByName(name, client)
}

// =============================================================================
// testEnv — общий setup для e2e-тестов.
// =============================================================================

type testEnv struct {
	t            *testing.T
	dir          string
	singboxPath  string
	backupDir    string
	cachePath    string
	logDir       string
	mock         *mockHTTPServer
	dnsServer    *dnsServer
	client       *http.Client
	parser       *vpnurl.Parser
	store        *singbox.Store
	validator    updater.ConfigValidator
	logger       *slog.Logger
	logCloser    io.Closer
	geoProviders []string
	geoByIP      map[string]string
}

// setupOpts параметризует setup.
type setupOpts struct {
	// Providers задаёт порядок geo-провайдеров. nil = default (все 6).
	Providers []string
	// DNSResolvers задаёт кастомные DNS-резолверы. nil = net.DefaultResolver.
	DNSResolvers []string
	// SubLinks — ссылки, возвращаемые /sub/subscription и /sub/plain.
	SubLinks []string
	// GeoByIP — маппинг IP -> страна (используется всеми провайдерами).
	GeoByIP map[string]string
	// ProviderStatus — маппинг provider -> HTTP status. status != 200 = ошибка.
	ProviderStatus map[string]int
}

func defaultProviders() []string {
	return []string{
		"ipapi_co", "ip_api_com", "ipwho_is",
		"api_2ip_me", "api_ip_sb", "freegeoip_app",
	}
}

func setup(t *testing.T, opts setupOpts) *testEnv {
	t.Helper()

	dir := t.TempDir()
	env := &testEnv{
		t:            t,
		dir:          dir,
		singboxPath:  filepath.Join(dir, "singbox.json"),
		backupDir:    filepath.Join(dir, "backups"),
		cachePath:    filepath.Join(dir, "cache.json"),
		logDir:       filepath.Join(dir, "logs"),
		geoProviders: opts.Providers,
		geoByIP:      opts.GeoByIP,
	}

	require.NoError(t, os.MkdirAll(env.backupDir, 0o755))
	require.NoError(t, os.MkdirAll(env.logDir, 0o755))

	// Копируем шаблон sing-box в temp dir.
	copyFile(t, "testdata/singbox-template.json", env.singboxPath)

	// Mock HTTP сервер.
	env.mock = newMockHTTPServer(t)
	if opts.GeoByIP != nil {
		for ip, country := range opts.GeoByIP {
			env.mock.setGeo(ip, country)
		}
	}
	if opts.ProviderStatus != nil {
		for p, s := range opts.ProviderStatus {
			env.mock.setProviderStatus(p, s)
		}
	}
	if opts.SubLinks != nil {
		env.mock.setSubscriptionLinks(opts.SubLinks)
	}

	// HTTP клиент с подменой URL.
	env.client = newRewritingClient(t, env.mock)

	// Логгер.
	log, closer, err := logger.New(env.logDir)
	require.NoError(t, err)
	env.logger = log
	env.logCloser = closer
	t.Cleanup(func() { _ = closer.Close() })

	// Опционально: mock DNS сервер.
	if len(opts.DNSResolvers) > 0 {
		ip := net.ParseIP(ipFromDNS).To4()
		env.dnsServer = newDNSServer(t, testDomain, ip)
	}

	env.parser = vpnurl.NewParser()
	env.store = &singbox.Store{}
	env.validator = singboxcli.NewNullChecker()
	return env
}

// buildUpdater собирает updater.Updater с реальными компонентами
// production-кода, но на mock инфраструктуре.
func (e *testEnv) buildUpdater(t *testing.T) *updater.Updater {
	t.Helper()

	geoProviderNames := e.geoProviders
	if geoProviderNames == nil {
		geoProviderNames = defaultProviders()
	}

	geoProviders := make([]ipserv.IPLookup, 0, len(geoProviderNames))
	for _, name := range geoProviderNames {
		p, err := providersBuild(t, name, e.client)
		require.NoError(t, err)
		geoProviders = append(geoProviders, p)
	}

	geoFallback := ipserv.NewFallback(geoProviders)
	geoCache := ipserv.NewFileCache(e.cachePath)
	geoService := ipserv.NewCachedIPLookup(geoFallback, geoCache)

	fetchers := map[config.SourceType]profile.LinkFetcher{
		config.SourceTypeSubscription: subscription.NewClient(e.client),
		config.SourceTypePlaintext:    plaintext.NewClient(e.client),
	}

	// DNS: либо net.DefaultResolver, либо цепочка с mock DNS.
	var dns resolver.IPResolver = net.DefaultResolver
	if e.dnsServer != nil {
		opts, err := resolver.OptionsFromURLs([]string{e.dnsServer.addr})
		require.NoError(t, err)
		dns = resolver.New(opts...)
	}

	return updater.NewUpdater(
		fetchers,
		dns,
		geoService,
		e.parser,
		e.store,
		e.validator,
		nil,
	)
}

// runContext возвращает context с логгером.
func (e *testEnv) runContext() context.Context {
	return logger.IntoContext(context.Background(), e.logger)
}

// config возвращает тестовый *config.Config.
// Сплит стран реалистичный: MULTI_WEST — западные (без России),
// MULTI_RU — только Россия. Это совпадает с конфигурацией из README
// и позволяет корректно проверять распределение прокси по секциям.
func (e *testEnv) buildConfig(t *testing.T) *config.Config {
	t.Helper()

	conf := &config.Config{
		CachePath:         e.cachePath,
		SingboxConfig:     e.singboxPath,
		BackupDir:         e.backupDir,
		MaxBackups:        5,
		MaxCacheSizeBytes: 1024 * 1024,
		LogDir:            e.logDir,
		URLTestDefaults: config.URLTestDefaults{
			URL:       "https://www.gstatic.com/generate_204",
			Interval:  "3m",
			Tolerance: 50,
		},
		GeoProviders: e.geoProviders,
	}

	conf.Sections = []config.Section{
		{
			Name: "MULTI_WEST",
			Countries: []string{
				countrySweden, countryNetherlands, countryUSA, countryLithuania,
			},
			Sources: []config.Source{
				{Type: config.SourceTypeSubscription, URLs: []string{e.mock.URL() + "/sub/subscription"}},
				{Type: config.SourceTypePlaintext, URLs: []string{e.mock.URL() + "/sub/plain"}},
			},
		},
		{
			Name:      "MULTI_RU",
			Countries: []string{countryRussia},
			Sources: []config.Source{
				{Type: config.SourceTypeSubscription, URLs: []string{e.mock.URL() + "/sub/subscription"}},
				{Type: config.SourceTypePlaintext, URLs: []string{e.mock.URL() + "/sub/plain"}},
			},
		},
	}
	return conf
}

// =============================================================================
// Helpers
// =============================================================================

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, data, 0o644))
}

// singboxJSON читает sing-box конфиг из e.singboxPath в map.
func (e *testEnv) singboxJSON(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(e.singboxPath)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

// outboundTags возвращает список тегов outbounds в исходном порядке.
func outboundTags(t *testing.T, sb map[string]any) []string {
	t.Helper()
	obs, ok := sb["outbounds"].([]any)
	require.True(t, ok, "outbounds not found or not array")
	tags := make([]string, 0, len(obs))
	for _, o := range obs {
		m, ok := o.(map[string]any)
		require.True(t, ok)
		if tag, ok := m["tag"].(string); ok {
			tags = append(tags, tag)
		} else {
			tags = append(tags, "<no-tag>")
		}
	}
	return tags
}

// outboundByTag возвращает первый outbound с указанным тегом.
func outboundByTag(t *testing.T, sb map[string]any, tag string) map[string]any {
	t.Helper()
	obs, _ := sb["outbounds"].([]any)
	for _, o := range obs {
		m, _ := o.(map[string]any)
		if m["tag"] == tag {
			return m
		}
	}
	t.Fatalf("outbound with tag %q not found", tag)
	return nil
}

// =============================================================================
// Tests
// =============================================================================

// TestE2E_FullCycle — полный цикл обновления с subscription/plaintext подписками
// и всеми 6 geo-провайдерами. Проверяет, что новые outbounds корректно
// генерируются, старые удаляются, структура секций соответствует
// эталону.
func TestE2E_FullCycle(t *testing.T) {
	t.Parallel()

	env := setup(t, setupOpts{
		SubLinks: fullSubscriptionLinks(),
		GeoByIP: map[string]string{
			ipSweden:      countrySweden,
			ipNetherlands: countryNetherlands,
			ipUSA:         countryUSA,
			ipLithuania:   countryLithuania,
			ipRussia:      countryRussia,
		},
	})

	ctx := env.runContext()
	u := env.buildUpdater(t)
	result, err := u.Run(ctx, env.buildConfig(t))
	require.NoError(t, err)

	// Assert: 21 уникальная ссылка в подписке, 20 распарсились (vmess skipped).
	// linksFetched — сырой счётчик запросов: 21 ссылка × 4 запроса (2 секции ×
	// 2 source-типа) = 84. Дедупликация происходит на этапе парсинга
	// (см. deduplicateCountryURLs в updater), urlsParsed — дедуплицированный
	// счётчик.
	assert.Equal(t, 84, result.LinksFetched, "raw fetch count (21 links × 4 requests)")
	assert.Equal(t, 20, result.URLsParsed, "deduplicated, vmess must be skipped")
	assert.True(t, result.Changed, "config must change on first run")
	assert.ElementsMatch(t, []string{"MULTI_WEST", "MULTI_RU"}, result.SectionsUpdated)

	// Assert: sing-box.json обновился.
	sb := env.singboxJSON(t)
	tags := outboundTags(t, sb)

	// Direct outbounds сохранены.
	assert.Contains(t, tags, "direct-out")
	assert.Contains(t, tags, "VGRU-out")

	// Stale outbounds удалены: проверяем по uuid из шаблона, а не по тегу,
	// потому что новые прокси легитимно используют теги MULTI_WEST-1-out и
	// MULTI_RU-1-out (первый слот в каждой секции).
	outs, _ := sb["outbounds"].([]any)
	for _, o := range outs {
		m, _ := o.(map[string]any)
		if uuid, ok := m["uuid"].(string); ok {
			assert.NotContains(t, uuid, "stale-uuid", "stale uuid must be removed")
		}
	}

	// countByPrefix включает proxy + urltest + selector: 16+2=18, 4+2=6.
	westTotal := countByPrefix(tags, "MULTI_WEST-")
	ruTotal := countByPrefix(tags, "MULTI_RU-")
	assert.Equal(t, 18, westTotal, "MULTI_WEST total (16 proxies + urltest + selector)")
	assert.Equal(t, 6, ruTotal, "MULTI_RU total (4 proxies + urltest + selector)")

	// urltest и selector для каждой секции.
	assert.Contains(t, tags, "MULTI_WEST-urltest-out")
	assert.Contains(t, tags, "MULTI_WEST-out")
	assert.Contains(t, tags, "MULTI_RU-urltest-out")
	assert.Contains(t, tags, "MULTI_RU-out")

	// urltest.outbounds содержит все proxy-теги секции.
	url := outboundByTag(t, sb, "MULTI_WEST-urltest-out")
	urlOuts, _ := url["outbounds"].([]any)
	assert.Len(t, urlOuts, 16, "urltest.outbounds must contain all 16 proxies")

	// selector.outbounds содержит 16 прокси + urltest = 17.
	sel := outboundByTag(t, sb, "MULTI_WEST-out")
	selOuts, _ := sel["outbounds"].([]any)
	assert.Len(t, selOuts, 17, "selector must contain 16 proxies + urltest")
	assert.Equal(t, "MULTI_WEST-urltest-out", sel["default"], "selector default")

	// vmess outbound не должен попасть в sing-box.
	for _, tag := range tags {
		assert.NotContains(t, tag, "vmess", "no vmess tag in outbounds")
	}

	// Cache дедуплицирует по IP: 5 уникальных IP → 5 вызовов ipapi_co.
	assert.Equal(t, 5, env.mock.hitCount("ipapi_co"), "5 unique IPs → 5 ipapi_co hits")
	assert.Equal(t, 0, env.mock.hitCount("ip_api_com"), "ip_api_com used only if ipapi_co failed")
}

// TestE2E_AllGeoProviders — каждый из 6 провайдеров отвечает успешно,
// и его IP-страна маппинг используется хотя бы раз. Проверяет, что
// провайдеры правильно инициализируются и доступны.
func TestE2E_AllGeoProviders(t *testing.T) {
	t.Parallel()

	// Каждый провайдер запускается отдельно с одной ссылкой.
	geoProviderNames := defaultProviders()
	for _, provName := range geoProviderNames {
		t.Run(provName, func(t *testing.T) {
			t.Parallel()
			env := setup(t, setupOpts{
				Providers: []string{provName},
				SubLinks:  ruVPNLinks(),
				GeoByIP: map[string]string{
					ipRussia: countryRussia,
				},
			})

			ctx := env.runContext()
			u := env.buildUpdater(t)
			result, err := u.Run(ctx, env.buildConfig(t))
			require.NoError(t, err)
			assert.Equal(t, 4, result.URLsParsed)
			assert.True(t, result.Changed)

			sb := env.singboxJSON(t)
			tags := outboundTags(t, sb)
			assert.Equal(t, 6, countByPrefix(tags, "MULTI_RU-"), "MULTI_RU outbounds (proxies + urltest + selector)")
			// ruVPNLinks() даёт 1 уникальный IP → cache дедуплицирует → 1 хит.
			assert.GreaterOrEqual(t, env.mock.hitCount(provName), 1, "provider must be hit")
		})
	}
}

// TestE2E_GeoProviderFallback — первые N провайдеров возвращают 500,
// fallback переходит к следующему. Финальный результат корректен.
func TestE2E_GeoProviderFallback(t *testing.T) {
	t.Parallel()

	env := setup(t, setupOpts{
		Providers: defaultProviders(),
		SubLinks:  ruVPNLinks(),
		GeoByIP: map[string]string{
			ipRussia: countryRussia,
		},
		ProviderStatus: map[string]int{
			"ipapi_co":   500,
			"ip_api_com": 500,
			"ipwho_is":   500,
			"api_2ip_me": 500,
			// api_ip_sb — рабочий, должен сработать.
		},
	})

	ctx := env.runContext()
	u := env.buildUpdater(t)
	result, err := u.Run(ctx, env.buildConfig(t))
	require.NoError(t, err)
	assert.Equal(t, 4, result.URLsParsed, "all 4 must be parsed after fallback")
	assert.True(t, result.Changed)

	// ruVPNLinks() даёт 1 уникальный IP, cache дедуплицирует → 1 хит на провайдер.
	// Первые 4 провайдера были вызваны (упали с 500).
	assert.GreaterOrEqual(t, env.mock.hitCount("ipapi_co"), 1)
	assert.GreaterOrEqual(t, env.mock.hitCount("ip_api_com"), 1)
	assert.GreaterOrEqual(t, env.mock.hitCount("ipwho_is"), 1)
	assert.GreaterOrEqual(t, env.mock.hitCount("api_2ip_me"), 1)
	// api_ip_sb сработал и вернул страну.
	assert.GreaterOrEqual(t, env.mock.hitCount("api_ip_sb"), 1)
	// freegeoip_app не должен был быть вызван.
	assert.Equal(t, 0, env.mock.hitCount("freegeoip_app"))
}

// TestE2E_VMessSkipped — подписка содержит vmess-ссылку, она пропускается
// и не попадает в sing-box.json.
func TestE2E_VMessSkipped(t *testing.T) {
	t.Parallel()

	env := setup(t, setupOpts{
		SubLinks: []string{
			vlessURL("00000000-0000-0000-0000-000000000001", ipSweden, "vless-1"),
			vmessLink(),
			trojanURL("pwd", ipRussia, "trojan-1"),
		},
		GeoByIP: map[string]string{
			ipSweden: countrySweden,
			ipRussia: countryRussia,
		},
	})

	ctx := env.runContext()
	u := env.buildUpdater(t)
	result, err := u.Run(ctx, env.buildConfig(t))
	require.NoError(t, err)

	// 3 уникальные ссылки в подписке, 2 распарсились (vmess skipped).
	// linksFetched — сырой счётчик: 3 × 4 запроса (2 секции × 2 типа) = 12.
	assert.Equal(t, 12, result.LinksFetched, "raw fetch count (3 links × 4 requests)")
	assert.Equal(t, 2, result.URLsParsed, "vmess must be skipped")
}

// TestE2E_NoChange — если sing-box.json уже содержит актуальные outbounds
// и они совпадают с текущей подпиской, обновление не должно изменять файл.
func TestE2E_NoChange(t *testing.T) {
	t.Parallel()

	env := setup(t, setupOpts{
		SubLinks: ruVPNLinks(),
		GeoByIP: map[string]string{
			ipRussia: countryRussia,
		},
	})

	ctx := env.runContext()
	u := env.buildUpdater(t)
	conf := env.buildConfig(t)

	// Первый запуск — обновляет sing-box.
	result1, err := u.Run(ctx, conf)
	require.NoError(t, err)
	assert.True(t, result1.Changed)

	// Запоминаем mtime файла.
	stat1, err := os.Stat(env.singboxPath)
	require.NoError(t, err)
	mtime1 := stat1.ModTime()

	// Второй запуск с теми же ссылками — изменений нет.
	time.Sleep(20 * time.Millisecond) // гарантируем разный mtime при записи
	result2, err := u.Run(ctx, conf)
	require.NoError(t, err)
	assert.False(t, result2.Changed, "second run with same data must be no-op")

	stat2, err := os.Stat(env.singboxPath)
	require.NoError(t, err)
	assert.Equal(t, mtime1, stat2.ModTime(), "file must not be rewritten when no change")
}

// TestE2E_OutboundsPreserved — outbounds без префикса секции (direct-out,
// VGRU-out) и другие не-секционные outbounds сохраняются при обновлении.
func TestE2E_OutboundsPreserved(t *testing.T) {
	t.Parallel()

	env := setup(t, setupOpts{
		SubLinks: ruVPNLinks(),
		GeoByIP: map[string]string{
			ipRussia: countryRussia,
		},
	})

	ctx := env.runContext()
	u := env.buildUpdater(t)
	_, err := u.Run(ctx, env.buildConfig(t))
	require.NoError(t, err)

	sb := env.singboxJSON(t)
	tags := outboundTags(t, sb)

	assert.Contains(t, tags, "direct-out", "direct-out preserved")
	assert.Contains(t, tags, "VGRU-out", "VGRU-out preserved")

	// Inbounds не изменились.
	inbounds, ok := sb["inbounds"].([]any)
	require.True(t, ok)
	assert.Len(t, inbounds, 2, "inbounds count unchanged")
}

// TestE2E_RulesetsPreserved — все rulesets из шаблона должны остаться
// в sing-box.json после обновления.
func TestE2E_RulesetsPreserved(t *testing.T) {
	t.Parallel()

	env := setup(t, setupOpts{
		SubLinks: ruVPNLinks(),
		GeoByIP: map[string]string{
			ipRussia: countryRussia,
		},
	})

	ctx := env.runContext()
	u := env.buildUpdater(t)
	_, err := u.Run(ctx, env.buildConfig(t))
	require.NoError(t, err)

	before := env.singboxJSON(t)
	route, _ := before["route"].(map[string]any)
	beforeRules, _ := route["rule_set"].([]any)

	// Повторный запуск с теми же ссылками — не должно затронуть rulesets.
	u2 := env.buildUpdater(t)
	_, err = u2.Run(ctx, env.buildConfig(t))
	require.NoError(t, err)

	after := env.singboxJSON(t)
	route, _ = after["route"].(map[string]any)
	afterRules, _ := route["rule_set"].([]any)

	require.Equal(t, len(beforeRules), len(afterRules), "rulesets count unchanged")
	beforeTags := ruleSetTags(t, beforeRules)
	afterTags := ruleSetTags(t, afterRules)
	assert.ElementsMatch(t, beforeTags, afterTags, "ruleset tags unchanged")
}

// TestE2E_DNSResolution — бонус. Доменная VPN-ссылка резолвится через
// кастомный DNS-сервер. URL попадает в правильную секцию sing-box.
func TestE2E_DNSResolution(t *testing.T) {
	t.Parallel()

	env := setup(t, setupOpts{
		SubLinks: domainSubscriptionLinks(),
		GeoByIP: map[string]string{
			ipSweden:      countrySweden,
			ipNetherlands: countryNetherlands,
			ipUSA:         countryUSA,
			ipLithuania:   countryLithuania,
			ipFromDNS:     countryDNSResolved,
		},
	})

	dnsIP := net.ParseIP(ipFromDNS).To4()
	env.dnsServer = newDNSServer(t, testDomain, dnsIP)
	opts, err := resolver.OptionsFromURLs([]string{env.dnsServer.addr})
	require.NoError(t, err)
	dns := resolver.New(opts...)

	// Собираем updater вручную с нашим DNS.
	ctx := env.runContext()
	geoProviderNames := defaultProviders()
	geoProviders := make([]ipserv.IPLookup, 0, len(geoProviderNames))
	for _, name := range geoProviderNames {
		p, err := providersBuild(t, name, env.client)
		require.NoError(t, err)
		geoProviders = append(geoProviders, p)
	}
	geoFallback := ipserv.NewFallback(geoProviders)
	geoCache := ipserv.NewFileCache(env.cachePath)
	geoService := ipserv.NewCachedIPLookup(geoFallback, geoCache)
	fetchers := map[config.SourceType]profile.LinkFetcher{
		config.SourceTypeSubscription: subscription.NewClient(env.client),
		config.SourceTypePlaintext:    plaintext.NewClient(env.client),
	}

	u := updater.NewUpdater(
		fetchers, dns, geoService,
		env.parser, env.store, env.validator,
		nil,
	)

	result, err := u.Run(ctx, env.buildConfig(t))
	require.NoError(t, err)

	// 5 уникальных ссылок в подписке: 4 (Sweden) + 1 domain-based.
	// linksFetched — сырой счётчик: 5 × 4 запроса (2 секции × 2 типа) = 20.
	assert.Equal(t, 20, result.LinksFetched, "raw fetch count (5 links × 4 requests)")
	assert.Equal(t, 5, result.URLsParsed, "domain URL must be resolved and parsed")
	assert.True(t, result.Changed)

	sb := env.singboxJSON(t)
	tags := outboundTags(t, sb)

	// 5 прокси + urltest + selector = 7 в MULTI_WEST (4 Sweden IP + 1 domain-resolved).
	westTotal := countByPrefix(tags, "MULTI_WEST-")
	assert.Equal(t, 7, westTotal, "MULTI_WEST total (5 proxies + urltest + selector)")

	// Доменная ссылка идёт последней (processed после IP-ссылок в country sort).
	var domainOutbound map[string]any
	outs, _ := sb["outbounds"].([]any)
	for _, o := range outs {
		m, _ := o.(map[string]any)
		tag, _ := m["tag"].(string)
		if tag == "MULTI_WEST-5-out" {
			domainOutbound = m
			break
		}
	}
	require.NotNil(t, domainOutbound, "domain-resolved outbound must be MULTI_WEST-5-out")

	// Парсер vless кладёт в server оригинальный хост из URL, не resolved IP.
	// DNS-резолв используется только для geo lookup, чтобы ссылка попала
	// в правильную секцию по стране. sing-box сам резолвит домен при
	// установлении соединения.
	assert.Equal(t, testDomain, domainOutbound["server"], "parser keeps original host; DNS used only for geo lookup")
}

// TestMockDNS_Smoke — smoke-тест самого mock DNS-сервера. Полезен при
// отладке инфраструктуры, изолирован от остальных тестов.
func TestMockDNS_Smoke(t *testing.T) {
	t.Parallel()

	want := net.ParseIP("192.0.2.99").To4()
	srv := newDNSServer(t, "foo.test", want)

	opts, err := resolver.OptionsFromURLs([]string{srv.addr})
	require.NoError(t, err)
	r := resolver.New(opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ips, err := r.LookupIP(ctx, "ip4", "foo.test")
	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.Equal(t, want.String(), ips[0].String())

	// NXDOMAIN для неизвестного домена.
	_, err = r.LookupIP(ctx, "ip4", "unknown.test")
	assert.Error(t, err, "unknown domain must return error")
}

// =============================================================================
// Small helpers (test-local)
// =============================================================================

func countByPrefix(tags []string, prefix string) int {
	n := 0
	for _, tag := range tags {
		if len(tag) >= len(prefix) && tag[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}

func ruleSetTags(t *testing.T, rules []any) []string {
	t.Helper()
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		m, _ := r.(map[string]any)
		if tag, ok := m["tag"].(string); ok {
			out = append(out, tag)
		}
	}
	slices.Sort(out)
	return out
}
