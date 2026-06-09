package updater

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
	"github.com/denisdubovitskiy/vpnconfig/internal/singbox"
	"github.com/denisdubovitskiy/vpnconfig/internal/vpnurl"
)

func TestUpdater_Run(t *testing.T) {
	t.Parallel()

	// Проверяем успешный сценарий обновления конфигурации.
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, "https://example.com/links").
			Return([]string{
				"vless://uuid@192.0.2.1:8444",
				"trojan://pass@198.51.100.1:2058",
				"vmess://base64",
			}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, "192.0.2.1").
			Return("Netherlands", nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, "198.51.100.1").
			Return("United States", nil)

		vlessOutbound := &vpnurl.VLESSOutbound{
			OutboundType: "vless",
			Server:       "192.0.2.1",
			ServerPort:   8444,
		}
		parser.EXPECT().
			Parse("vless://uuid@192.0.2.1:8444").
			Return(vlessOutbound, nil)

		trojanOutbound := &vpnurl.TrojanOutbound{
			OutboundType: "trojan",
			Server:       "198.51.100.1",
			ServerPort:   2058,
		}
		parser.EXPECT().
			Parse("trojan://pass@198.51.100.1:2058").
			Return(trojanOutbound, nil)

		singboxCfg := &singbox.Config{}
		configStore.EXPECT().
			LoadConfig("./singbox.json").
			Return(singboxCfg, nil)

		configStore.EXPECT().
			CreateBackup("./singbox.json").
			Return("./singbox.json.backup_20260101_120000", nil)

		configStore.EXPECT().
			SaveConfig("./singbox.json", singboxCfg).
			Return(nil)

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			URLTestDefaults: config.URLTestDefaults{
				URL:       "https://www.gstatic.com/generate_204",
				Interval:  "3m",
				Tolerance: 50,
			},
			Sections: []config.Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands", "United States"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		result, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.NoError(t, err)
		assert.Equal(t, 3, result.LinksFetched)
		assert.Equal(t, 2, result.URLsParsed)
		assert.Equal(t, map[string]int{
			"Netherlands":   1,
			"United States": 1,
		}, result.CountriesFound)
		assert.Equal(t, []string{"MULTI_WEST"}, result.SectionsUpdated)
		assert.Equal(t, "./singbox.json.backup_20260101_120000", result.BackupPath)
		assert.True(t, result.Changed)
	})

	// Проверяем ошибку при получении ссылок.
	t.Run("fetch links error", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, "https://example.com/links").
			Return(nil, errors.New("network error"))

		singboxCfg := &singbox.Config{}
		configStore.EXPECT().
			LoadConfig(mock.Anything).
			Return(singboxCfg, nil)

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			Sections: []config.Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		result, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.NoError(t, err)
		assert.False(t, result.Changed)
		assert.Empty(t, result.SectionsUpdated)
	})

	// Проверяем ошибку при загрузке конфигурации.
	t.Run("load config error", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, mock.Anything).
			Return([]string{"vless://uuid@192.0.2.1:8444"}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, mock.Anything).
			Return("Netherlands", nil)

		parser.EXPECT().
			Parse(mock.Anything).
			Return(&vpnurl.VLESSOutbound{OutboundType: "vless"}, nil)

		configStore.EXPECT().
			LoadConfig("./singbox.json").
			Return(nil, errors.New("file not found"))

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			Sections: []config.Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		_, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load singbox config")
	})

	// Проверяем ошибку при создании бэкапа.
	t.Run("create backup error", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, mock.Anything).
			Return([]string{"vless://uuid@192.0.2.1:8444"}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, mock.Anything).
			Return("Netherlands", nil)

		parser.EXPECT().
			Parse(mock.Anything).
			Return(&vpnurl.VLESSOutbound{OutboundType: "vless"}, nil)

		singboxCfg := &singbox.Config{}
		configStore.EXPECT().
			LoadConfig(mock.Anything).
			Return(singboxCfg, nil)

		configStore.EXPECT().
			CreateBackup(mock.Anything).
			Return("", errors.New("permission denied"))

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			Sections: []config.Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		_, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create backup")
	})

	// Проверяем ошибку при сохранении конфигурации.
	t.Run("save config error", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, mock.Anything).
			Return([]string{"vless://uuid@192.0.2.1:8444"}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, mock.Anything).
			Return("Netherlands", nil)

		parser.EXPECT().
			Parse(mock.Anything).
			Return(&vpnurl.VLESSOutbound{OutboundType: "vless"}, nil)

		singboxCfg := &singbox.Config{}
		configStore.EXPECT().
			LoadConfig(mock.Anything).
			Return(singboxCfg, nil)

		configStore.EXPECT().
			CreateBackup(mock.Anything).
			Return("./backup", nil)

		configStore.EXPECT().
			SaveConfig(mock.Anything, mock.Anything).
			Return(errors.New("disk full"))

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			Sections: []config.Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		_, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "save singbox config")
	})

	// Проверяем, что при отсутствии изменений конфиг не сохраняется.
	t.Run("no changes detected", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, mock.Anything).
			Return([]string{"vless://uuid@192.0.2.1:8444"}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, mock.Anything).
			Return("Netherlands", nil)

		parser.EXPECT().
			Parse(mock.Anything).
			Return(&vpnurl.VLESSOutbound{OutboundType: "vless"}, nil)

		singboxCfg := &singbox.Config{
			Outbounds: []singbox.Outbound{
				{"type": "direct", "tag": "direct-out"},
			},
		}
		configStore.EXPECT().
			LoadConfig(mock.Anything).
			Return(singboxCfg, nil)

		// SaveConfig и CreateBackup не должны вызываться.

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			Sections: []config.Section{
				{
					Name:      "MULTI_RU",
					Countries: []string{"Russia"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		result, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.NoError(t, err)
		assert.False(t, result.Changed)
		assert.Empty(t, result.SectionsUpdated)
		assert.Empty(t, result.BackupPath)
	})

	// Проверяем, что vmess URL пропускаются.
	t.Run("skips vmess urls", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, mock.Anything).
			Return([]string{
				"vmess://base64encoded",
				"vless://uuid@192.0.2.1:8444",
			}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, "192.0.2.1").
			Return("Netherlands", nil)

		parser.EXPECT().
			Parse("vless://uuid@192.0.2.1:8444").
			Return(&vpnurl.VLESSOutbound{OutboundType: "vless"}, nil)

		singboxCfg := &singbox.Config{}
		configStore.EXPECT().
			LoadConfig(mock.Anything).
			Return(singboxCfg, nil)

		configStore.EXPECT().
			CreateBackup(mock.Anything).
			Return("./backup", nil)

		configStore.EXPECT().
			SaveConfig(mock.Anything, mock.Anything).
			Return(nil)

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			Sections: []config.Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		result, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.NoError(t, err)
		assert.Equal(t, 2, result.LinksFetched)
		assert.Equal(t, 1, result.URLsParsed)
		assert.True(t, result.Changed)
	})

	// Проверяем, что секция без outbounds пропускается.
	t.Run("no outbounds for section", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, mock.Anything).
			Return([]string{"vless://uuid@192.0.2.1:8444"}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, mock.Anything).
			Return("Netherlands", nil)

		parser.EXPECT().
			Parse(mock.Anything).
			Return(&vpnurl.VLESSOutbound{OutboundType: "vless"}, nil)

		singboxCfg := &singbox.Config{}
		configStore.EXPECT().
			LoadConfig(mock.Anything).
			Return(singboxCfg, nil)

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			Sections: []config.Section{
				{
					Name:      "MULTI_RU",
					Countries: []string{"Russia"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
				},
			},
		}

		// act
		result, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.NoError(t, err)
		assert.Empty(t, result.SectionsUpdated)
		assert.False(t, result.Changed)
	})

	// Проверяем переопределение URLTest настроек в секции.
	t.Run("section urltest override", func(t *testing.T) {
		t.Parallel()

		// arrange
		fetcher := NewMockLinkFetcher(t)
		geoIP := NewMockGeoIPService(t)
		parser := NewMockVPNParser(t)
		configStore := NewMockConfigStore(t)

		fetcher.EXPECT().
			FetchLinks(mock.Anything, mock.Anything).
			Return([]string{"vless://uuid@192.0.2.1:8444"}, nil)

		geoIP.EXPECT().
			CountryName(mock.Anything, mock.Anything).
			Return("Netherlands", nil)

		parser.EXPECT().
			Parse(mock.Anything).
			Return(&vpnurl.VLESSOutbound{OutboundType: "vless"}, nil)

		singboxCfg := &singbox.Config{}
		configStore.EXPECT().
			LoadConfig(mock.Anything).
			Return(singboxCfg, nil)

		configStore.EXPECT().
			CreateBackup(mock.Anything).
			Return("./backup", nil)

		configStore.EXPECT().
			SaveConfig(mock.Anything, mock.Anything).
			Return(nil)

		fetchers := map[config.SourceType]LinkFetcher{
			config.SourceTypeSubscription: fetcher,
		}
		updater := NewUpdater(fetchers, nil, geoIP, parser, configStore, nil)

		customURL := "https://custom-url.com"
		customInterval := "5m"
		customTolerance := 100

		cfg := &config.Config{
			SingboxConfig: "./singbox.json",
			URLTestDefaults: config.URLTestDefaults{
				URL:       "https://default-url.com",
				Interval:  "3m",
				Tolerance: 50,
			},
			Sections: []config.Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []config.Source{
						{Type: config.SourceTypeSubscription, URLs: []string{"https://example.com/links"}},
					},
					URLTest: &config.URLTestDefaults{
						URL:       customURL,
						Interval:  customInterval,
						Tolerance: customTolerance,
					},
				},
			},
		}

		// act
		result, err := updater.Run(logger.IntoContext(t.Context(), logger.Silent()), cfg)

		// assert
		require.NoError(t, err)
		assert.Equal(t, []string{"MULTI_WEST"}, result.SectionsUpdated)
		assert.True(t, result.Changed)

		// Проверяем, что urltest outbound использует переопределённые настройки.
		require.Len(t, singboxCfg.Outbounds, 3) // proxy + urltest + selector
		urltestOutbound := singboxCfg.Outbounds[1]
		assert.Equal(t, "urltest", urltestOutbound.Type())
		assert.Equal(t, customURL, urltestOutbound["url"])
		assert.Equal(t, customInterval, urltestOutbound["interval"])
		assert.Equal(t, customTolerance, urltestOutbound["tolerance"])
	})
}

func TestDeduplicateCountryURLs(t *testing.T) {
	t.Parallel()

	// Проверяем, что дубликаты внутри одной страны удаляются с сохранением порядка.
	t.Run("removes duplicates within country", func(t *testing.T) {
		t.Parallel()

		// arrange
		input := map[string][]string{
			"Netherlands": {
				"vless://uuid@192.0.2.1:8444",
				"trojan://pass@192.0.2.2:2058",
				"vless://uuid@192.0.2.1:8444",
			},
		}

		// act
		deduplicateCountryURLs(input)

		// assert
		assert.Equal(t, []string{
			"vless://uuid@192.0.2.1:8444",
			"trojan://pass@192.0.2.2:2058",
		}, input["Netherlands"])
	})

	// Проверяем, что дедупликация работает независимо для каждой страны.
	t.Run("deduplicates per country", func(t *testing.T) {
		t.Parallel()

		// arrange
		input := map[string][]string{
			"Netherlands": {"url-a", "url-b", "url-a"},
			"Germany":     {"url-c", "url-c", "url-d"},
		}

		// act
		deduplicateCountryURLs(input)

		// assert
		assert.Equal(t, []string{"url-a", "url-b"}, input["Netherlands"])
		assert.Equal(t, []string{"url-c", "url-d"}, input["Germany"])
	})

	// Проверяем, что пустая мапа обрабатывается без ошибок.
	t.Run("empty map", func(t *testing.T) {
		t.Parallel()

		// arrange
		input := map[string][]string{}

		// act
		deduplicateCountryURLs(input)

		// assert
		assert.Empty(t, input)
	})

	// Проверяем, что отсутствие дубликатов не меняет данные.
	t.Run("no duplicates", func(t *testing.T) {
		t.Parallel()

		// arrange
		input := map[string][]string{
			"Netherlands": {"url-a", "url-b"},
		}

		// act
		deduplicateCountryURLs(input)

		// assert
		assert.Equal(t, []string{"url-a", "url-b"}, input["Netherlands"])
	})
}

func TestParseIPFromVpnURL(t *testing.T) {
	t.Parallel()

	// Проверяем парсинг vless URL.
	t.Run("vless url", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vless://uuid@192.0.2.2:8444?security=reality"
		want := "192.0.2.2"

		// act
		got, err := parseIPFromVpnURL(context.Background(), nil, url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	// Проверяем парсинг trojan URL.
	t.Run("trojan url", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "trojan://password@198.51.100.1:2058?security=tls"
		want := "198.51.100.1"

		// act
		got, err := parseIPFromVpnURL(context.Background(), nil, url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	// Проверяем парсинг ss URL.
	t.Run("ss url", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "ss://base64@192.0.2.1:2060#comment"
		want := "192.0.2.1"

		// act
		got, err := parseIPFromVpnURL(context.Background(), nil, url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	// Проверяем, что vmess URL возвращает ошибку.
	t.Run("vmess url returns error", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vmess://some=="

		// act
		_, err := parseIPFromVpnURL(context.Background(), nil, url)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vmess is not supported")
	})

	// Проверяем обработку невалидного URL.
	t.Run("invalid url no scheme", func(t *testing.T) {
		t.Parallel()

		// act
		_, err := parseIPFromVpnURL(context.Background(), nil, "not-a-url")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no scheme")
	})

	// Проверяем обработку пустого хоста.
	t.Run("empty host", func(t *testing.T) {
		t.Parallel()

		// act
		_, err := parseIPFromVpnURL(context.Background(), nil, "vless://uuid@:8444")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty host")
	})

	// Проверяем успешный DNS-резолв доменного хоста.
	t.Run("resolves domain via DNS", func(t *testing.T) {
		t.Parallel()

		// arrange
		dns := &fakeDNSResolver{
			lookup: func(_ context.Context, _, _ string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			},
		}

		// act
		got, err := parseIPFromVpnURL(context.Background(), dns, "vless://uuid@example.com:8444")

		// assert
		require.NoError(t, err)
		assert.Equal(t, "93.184.216.34", got)
	})

	// Проверяем ошибку DNS-резолва.
	t.Run("dns resolution error", func(t *testing.T) {
		t.Parallel()

		// arrange
		wantErr := errors.New("no such host")
		dns := &fakeDNSResolver{
			lookup: func(_ context.Context, _, _ string) ([]net.IP, error) {
				return nil, wantErr
			},
		}

		// act
		_, err := parseIPFromVpnURL(context.Background(), dns, "vless://uuid@nonexistent.invalid:8444")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolve domain")
		assert.ErrorIs(t, err, wantErr)
	})
}

func TestUpdater_CleanupCacheIfNeeded(t *testing.T) {
	t.Parallel()

	u := NewUpdater(nil, nil, nil, nil, nil, nil)

	// Проверяем очистку кэша при превышении размера.
	t.Run("clears cache when size exceeds limit", func(t *testing.T) {
		t.Parallel()

		// arrange
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "cache.json")
		data := make([]byte, 100)
		require.NoError(t, os.WriteFile(cachePath, data, 0o644))

		// act
		err := u.cleanupCacheIfNeeded(logger.IntoContext(t.Context(), logger.Silent()), cachePath, 50)

		// assert
		require.NoError(t, err)
		content, err := os.ReadFile(cachePath)
		require.NoError(t, err)
		assert.Equal(t, "{}", string(content))
	})

	// Проверяем, что кэш не очищается, если размер в пределах лимита.
	t.Run("does not clear cache when size within limit", func(t *testing.T) {
		t.Parallel()

		// arrange
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "cache.json")
		data := []byte(`{"key": "value"}`)
		require.NoError(t, os.WriteFile(cachePath, data, 0o644))

		// act
		err := u.cleanupCacheIfNeeded(logger.IntoContext(t.Context(), logger.Silent()), cachePath, 1000)

		// assert
		require.NoError(t, err)
		content, err := os.ReadFile(cachePath)
		require.NoError(t, err)
		assert.Equal(t, string(data), string(content))
	})

	// Проверяем, что ничего не делается при нулевом лимите.
	t.Run("does nothing when limit is zero", func(t *testing.T) {
		t.Parallel()

		// arrange
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "cache.json")
		data := []byte(`{"key": "value"}`)
		require.NoError(t, os.WriteFile(cachePath, data, 0o644))

		// act
		err := u.cleanupCacheIfNeeded(logger.IntoContext(t.Context(), logger.Silent()), cachePath, 0)

		// assert
		require.NoError(t, err)
		content, err := os.ReadFile(cachePath)
		require.NoError(t, err)
		assert.Equal(t, string(data), string(content))
	})
}

func TestUpdater_CreateBackup(t *testing.T) {
	t.Parallel()

	// Проверяем создание бэкапа в указанной директории.
	t.Run("creates backup in specified directory", func(t *testing.T) {
		t.Parallel()

		// arrange
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "singbox.json")
		backupDir := filepath.Join(tmpDir, "backups")
		require.NoError(t, os.WriteFile(configPath, []byte("test config"), 0o644))

		configStore := NewMockConfigStore(t)
		u := NewUpdater(nil, nil, nil, nil, configStore, nil)

		// act
		backupPath, err := u.createBackup(configPath, backupDir)

		// assert
		require.NoError(t, err)
		assert.Contains(t, backupPath, backupDir)
		assert.FileExists(t, backupPath)
		content, err := os.ReadFile(backupPath)
		require.NoError(t, err)
		assert.Equal(t, "test config", string(content))
	})

	// Проверяем, что при пустой директории используется ConfigStore.
	t.Run("uses config store when backup dir is empty", func(t *testing.T) {
		t.Parallel()

		// arrange
		configStore := NewMockConfigStore(t)
		configStore.EXPECT().
			CreateBackup("./singbox.json").
			Return("./singbox.json.backup_20260101_120000", nil)

		u := NewUpdater(nil, nil, nil, nil, configStore, nil)

		// act
		backupPath, err := u.createBackup("./singbox.json", "")

		// assert
		require.NoError(t, err)
		assert.Equal(t, "./singbox.json.backup_20260101_120000", backupPath)
	})
}

func TestUpdater_CleanupOldBackups(t *testing.T) {
	t.Parallel()

	u := NewUpdater(nil, nil, nil, nil, nil, nil)

	// Проверяем удаление старых бэкапов при превышении лимита.
	t.Run("removes old backups when exceeding max", func(t *testing.T) {
		t.Parallel()

		// arrange
		tmpDir := t.TempDir()
		for i := 0; i < 5; i++ {
			path := filepath.Join(tmpDir, fmt.Sprintf("singbox.json.backup_2026010%d_120000", i+1))
			require.NoError(t, os.WriteFile(path, []byte("backup"), 0o644))
		}

		// act
		err := u.cleanupOldBackups(logger.IntoContext(t.Context(), logger.Silent()), tmpDir, 3)

		// assert
		require.NoError(t, err)
		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Len(t, entries, 3)
	})

	// Проверяем, что ничего не удаляется, если бэкапов не больше лимита.
	t.Run("does nothing when backups within limit", func(t *testing.T) {
		t.Parallel()

		// arrange
		tmpDir := t.TempDir()
		for i := 0; i < 3; i++ {
			path := filepath.Join(tmpDir, fmt.Sprintf("singbox.json.backup_2026010%d_120000", i+1))
			require.NoError(t, os.WriteFile(path, []byte("backup"), 0o644))
		}

		// act
		err := u.cleanupOldBackups(logger.IntoContext(t.Context(), logger.Silent()), tmpDir, 5)

		// assert
		require.NoError(t, err)
		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Len(t, entries, 3)
	})

	// Проверяем, что ничего не делается при нулевом лимите.
	t.Run("does nothing when max backups is zero", func(t *testing.T) {
		t.Parallel()

		// arrange
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "singbox.json.backup_20260101_120000")
		require.NoError(t, os.WriteFile(path, []byte("backup"), 0o644))

		// act
		err := u.cleanupOldBackups(logger.IntoContext(t.Context(), logger.Silent()), tmpDir, 0)

		// assert
		require.NoError(t, err)
		assert.FileExists(t, path)
	})
}

func TestUpdater_SaveConfigWithValidation(t *testing.T) {
	t.Parallel()

	// Проверяем, что при nil validator сохраняется напрямую без tmp файла.
	t.Run("saves directly when validator is nil", func(t *testing.T) {
		t.Parallel()

		// arrange
		configPath := filepath.Join(t.TempDir(), "singbox.json")
		cfg := &singbox.Config{Outbounds: []singbox.Outbound{{"type": "direct", "tag": "out"}}}
		store := &singbox.Store{}
		u := NewUpdater(nil, nil, nil, nil, store, nil)

		// act
		err := u.saveConfigWithValidation(logger.IntoContext(t.Context(), logger.Silent()), configPath, cfg)

		// assert
		require.NoError(t, err)

		loaded, err := singbox.LoadConfig(configPath)
		require.NoError(t, err)
		require.Len(t, loaded.Outbounds, 1)
		assert.Equal(t, "out", loaded.Outbounds[0].Tag())

		assert.NoFileExists(t, configPath+".tmp")
	})

	// Проверяем, что при проходящей валидации tmp переименовывается в оригинал.
	t.Run("renames tmp to original on validation success", func(t *testing.T) {
		t.Parallel()

		// arrange
		configPath := filepath.Join(t.TempDir(), "singbox.json")
		cfg := &singbox.Config{Outbounds: []singbox.Outbound{{"type": "direct", "tag": "out"}}}
		store := &singbox.Store{}
		validator := NewMockConfigValidator(t)
		validator.EXPECT().
			CheckConfig(mock.Anything, configPath+".tmp").
			Return(nil)

		u := NewUpdater(nil, nil, nil, nil, store, validator)

		// act
		err := u.saveConfigWithValidation(logger.IntoContext(t.Context(), logger.Silent()), configPath, cfg)

		// assert
		require.NoError(t, err)

		loaded, err := singbox.LoadConfig(configPath)
		require.NoError(t, err)
		require.Len(t, loaded.Outbounds, 1)
		assert.Equal(t, "out", loaded.Outbounds[0].Tag())

		assert.NoFileExists(t, configPath+".tmp")
	})

	// Проверяем, что при ошибке валидации tmp удаляется и оригинал не меняется.
	t.Run("removes tmp on validation error", func(t *testing.T) {
		t.Parallel()

		// arrange
		configPath := filepath.Join(t.TempDir(), "singbox.json")
		cfg := &singbox.Config{Outbounds: []singbox.Outbound{{"type": "direct", "tag": "out"}}}
		store := &singbox.Store{}
		wantErr := errors.New("invalid config")
		validator := NewMockConfigValidator(t)
		validator.EXPECT().
			CheckConfig(mock.Anything, configPath+".tmp").
			Return(wantErr)

		u := NewUpdater(nil, nil, nil, nil, store, validator)

		// act
		err := u.saveConfigWithValidation(logger.IntoContext(t.Context(), logger.Silent()), configPath, cfg)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)

		assert.NoFileExists(t, configPath+".tmp")
		assert.NoFileExists(t, configPath)
	})

	// Проверяем ошибку при невозможности записать tmp файл.
	t.Run("error when save tmp fails", func(t *testing.T) {
		t.Parallel()

		// arrange
		// Родительская директория не существует — os.WriteFile вернёт ошибку.
		configPath := filepath.Join(t.TempDir(), "nonexistent_subdir", "singbox.json")
		cfg := &singbox.Config{Outbounds: []singbox.Outbound{{"type": "direct", "tag": "out"}}}
		store := &singbox.Store{}
		validator := NewMockConfigValidator(t)
		// CheckConfig опционален: до валидации дело не дойдёт, но если
		// поведение изменится, тест всё равно пройдёт.
		validator.EXPECT().
			CheckConfig(mock.Anything, mock.Anything).
			Return(nil).
			Maybe()

		u := NewUpdater(nil, nil, nil, nil, store, validator)

		// act
		err := u.saveConfigWithValidation(logger.IntoContext(t.Context(), logger.Silent()), configPath, cfg)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "save temp config")
	})
}

// fakeDNSResolver — тестовая реализация DNSResolver через замыкание.
// Используется вместо mockery-мока, потому что _test.go-файлы с моками
// недоступны за пределами своего пакета.
type fakeDNSResolver struct {
	lookup func(ctx context.Context, network, host string) ([]net.IP, error)
}

func (f *fakeDNSResolver) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f.lookup(ctx, network, host)
}
