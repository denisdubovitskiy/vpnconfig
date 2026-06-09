package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	// Проверяем успешную загрузку конфига.
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
cache_path: "/tmp/cache.json"
sections:
  - name: "Europe"
    countries:
      - "Germany"
      - "France"
    sources:
      - type: subscription
        urls:
          - "https://example.com/subscription"
`)
		err := os.WriteFile(path, data, 0o644)
		require.NoError(t, err)

		// act
		cfg, err := Load(path)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "/tmp/cache.json", cfg.CachePath)
		require.Len(t, cfg.Sections, 1)
		assert.Equal(t, "Europe", cfg.Sections[0].Name)
		assert.Equal(t, []string{"Germany", "France"}, cfg.Sections[0].Countries)
		require.Len(t, cfg.Sections[0].Sources, 1)
		assert.Equal(t, SourceTypeSubscription, cfg.Sections[0].Sources[0].Type)
		assert.Equal(t, []string{"https://example.com/subscription"}, cfg.Sections[0].Sources[0].URLs)
	})

	// Проверяем ошибку при отсутствии файла.
	t.Run("file not found", func(t *testing.T) {
		t.Parallel()

		// act
		_, err := Load("/nonexistent/config.yaml")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read config file")
	})

	// Проверяем ошибку при невалидном YAML.
	t.Run("invalid yaml", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		err := os.WriteFile(path, []byte("not: valid: yaml: ["), 0o644)
		require.NoError(t, err)

		// act
		_, err = Load(path)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse config")
	})

	// Проверяем ошибку валидации при неизвестном провайдере.
	t.Run("unknown geo provider", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
cache_path: "/tmp/cache.json"
geo_providers:
  - "unknown_provider"
sections:
  - name: "Europe"
    countries: ["Germany"]
    sources:
      - type: subscription
        urls:
          - "https://example.com/subscription"
`)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		// act
		_, err := Load(path)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown geo provider")
		assert.Contains(t, err.Error(), "unknown_provider")
	})

	// Проверяем, что валидный список провайдеров проходит валидацию.
	t.Run("valid geo providers", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
cache_path: "/tmp/cache.json"
geo_providers:
  - "ip_api_com"
  - "ipwho_is"
sections:
  - name: "Europe"
    countries: ["Germany"]
    sources:
      - type: subscription
        urls:
          - "https://example.com/subscription"
`)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		// act
		cfg, err := Load(path)

		// assert
		require.NoError(t, err)
		assert.Equal(t, []string{"ip_api_com", "ipwho_is"}, cfg.GeoProviders)
	})
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	// Проверяем, что пустой GeoProviders — валидный.
	t.Run("empty geo providers is valid", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{}

		err := cfg.Validate()

		require.NoError(t, err)
	})

	// Проверяем, что все встроенные провайдеры проходят валидацию.
	t.Run("all default names are valid", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{
			GeoProviders: []string{
				"ipapi_co",
				"ip_api_com",
				"ipwho_is",
				"api_2ip_me",
				"api_ip_sb",
				"freegeoip_app",
			},
		}

		// act
		err := cfg.Validate()

		// assert
		require.NoError(t, err)
	})

	// Проверяем ошибку при невалидном имени.
	t.Run("invalid name", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			GeoProviders: []string{"ipapi_co", "typo_provider"},
		}

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "typo_provider")
	})

	// Проверяем ошибку при mmdb.enabled=true и пустом database_path.
	t.Run("mmdb enabled without path fails", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{MMDB: &MMDBConfig{Enabled: true}}

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "mmdb.database_path")
	})

	// Проверяем, что mmdb.enabled=false с пустым path — валидно.
	t.Run("mmdb disabled without path is valid", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{MMDB: &MMDBConfig{Enabled: false}}

		err := cfg.Validate()

		require.NoError(t, err)
	})

	// Проверяем, что nil mmdb — валидно.
	t.Run("nil mmdb is valid", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{}

		err := cfg.Validate()

		require.NoError(t, err)
	})

	// Проверяем, что mmdb.enabled=true с заполненным path — валидно.
	t.Run("mmdb enabled with path is valid", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			MMDB: &MMDBConfig{
				Enabled:      true,
				DatabasePath: "/tmp/test.mmdb",
			},
		}

		err := cfg.Validate()

		require.NoError(t, err)
	})

	// Проверяем ошибку при секции без источников.
	t.Run("section without sources fails", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			Sections: []Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources:   []Source{},
				},
			},
		}

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have at least one source")
		assert.Contains(t, err.Error(), "MULTI_WEST")
	})

	// Проверяем ошибку при неизвестном типе источника.
	t.Run("unknown source type fails", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			Sections: []Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []Source{
						{Type: "unknown", URLs: []string{"https://example.com"}},
					},
				},
			},
		}

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown source type")
		assert.Contains(t, err.Error(), "unknown")
	})

	// Проверяем ошибку при источнике без URL.
	t.Run("source without urls fails", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			Sections: []Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []Source{
						{Type: SourceTypeSubscription, URLs: []string{}},
					},
				},
			},
		}

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have at least one URL")
	})

	// Проверяем, что валидная секция с источниками проходит валидацию.
	t.Run("valid section with sources is valid", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			Sections: []Section{
				{
					Name:      "MULTI_WEST",
					Countries: []string{"Netherlands"},
					Sources: []Source{
						{Type: SourceTypeSubscription, URLs: []string{"https://example.com/subscription"}},
						{Type: SourceTypePlaintext, URLs: []string{"https://example.com/plaintext"}},
					},
				},
			},
		}

		err := cfg.Validate()

		require.NoError(t, err)
	})
}

func TestConfig_EffectiveGeoProviders(t *testing.T) {
	t.Parallel()

	// Если GeoProviders пусто — возвращаются дефолтные имена.
	t.Run("empty returns defaults", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{}

		got := cfg.EffectiveGeoProviders()

		assert.NotEmpty(t, got)
		assert.Contains(t, got, "ip_api_com")
	})

	// Если GeoProviders задан — возвращается как есть.
	t.Run("custom returns as is", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			GeoProviders: []string{"ipwho_is", "ip_api_com"},
		}

		got := cfg.EffectiveGeoProviders()

		assert.Equal(t, []string{"ipwho_is", "ip_api_com"}, got)
	})
}

func TestConfig_MMDBEnabled(t *testing.T) {
	t.Parallel()

	t.Run("nil mmdb returns false", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{}

		assert.False(t, cfg.MMDBEnabled())
	})

	t.Run("disabled mmdb returns false", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{MMDB: &MMDBConfig{Enabled: false}}

		assert.False(t, cfg.MMDBEnabled())
	})

	t.Run("enabled mmdb returns true", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{MMDB: &MMDBConfig{Enabled: true, DatabasePath: "/tmp/test.mmdb"}}

		assert.True(t, cfg.MMDBEnabled())
	})
}

func TestMMDBConfig_EffectiveDownloadURL(t *testing.T) {
	t.Parallel()

	// Если DownloadURL не задан — используется default.
	t.Run("empty uses default", func(t *testing.T) {
		t.Parallel()

		cfg := &MMDBConfig{}

		assert.Equal(t, DefaultMMDBDownloadURL, cfg.EffectiveDownloadURL())
	})

	// Если DownloadURL задан — возвращается как есть.
	t.Run("custom returns as is", func(t *testing.T) {
		t.Parallel()

		const customURL = "https://example.com/custom.mmdb"
		cfg := &MMDBConfig{DownloadURL: customURL}

		assert.Equal(t, customURL, cfg.EffectiveDownloadURL())
	})
}

func TestLoad_MMDBConfig(t *testing.T) {
	t.Parallel()

	// Проверяем загрузку MMDB-секции.
	t.Run("loads mmdb section", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
cache_path: "/tmp/cache.json"
mmdb:
  enabled: true
  database_path: "/opt/mmdb/GeoLite2-Country.mmdb"
  download_url: "https://example.com/custom.mmdb"
sections:
  - name: "Europe"
    countries: ["Germany"]
    sources:
      - type: subscription
        urls:
          - "https://example.com/subscription"
`)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		// act
		cfg, err := Load(path)

		// assert
		require.NoError(t, err)
		require.NotNil(t, cfg.MMDB)
		assert.True(t, cfg.MMDB.Enabled)
		assert.Equal(t, "/opt/mmdb/GeoLite2-Country.mmdb", cfg.MMDB.DatabasePath)
		assert.Equal(t, "https://example.com/custom.mmdb", cfg.MMDB.DownloadURL)
	})

	// Проверяем, что без секции mmdb провайдер выключен.
	t.Run("without mmdb section is disabled", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
cache_path: "/tmp/cache.json"
sections:
  - name: "Europe"
    countries: ["Germany"]
    sources:
      - type: subscription
        urls:
          - "https://example.com/subscription"
`)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		// act
		cfg, err := Load(path)

		// assert
		require.NoError(t, err)
		assert.False(t, cfg.MMDBEnabled())
	})

	// Проверяем, что при отсутствии max_age в YAML применяется дефолт 168h.
	t.Run("applies default max_age when unset", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
cache_path: "/tmp/cache.json"
mmdb:
  enabled: true
  database_path: "/opt/mmdb/GeoLite2-Country.mmdb"
sections:
  - name: "Europe"
    countries: ["Germany"]
    sources:
      - type: subscription
        urls:
          - "https://example.com/subscription"
`)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		// act
		cfg, err := Load(path)

		// assert
		require.NoError(t, err)
		require.NotNil(t, cfg.MMDB)
		assert.Equal(t, DefaultMMDBMaxAge, cfg.MMDB.MaxAge.Duration)
	})
}

func TestConfig_Normalize(t *testing.T) {
	t.Parallel()

	t.Run("applies default when max_age is 0", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{MMDB: &MMDBConfig{Enabled: true, DatabasePath: "/tmp/test.mmdb"}}

		cfg.Normalize()

		assert.Equal(t, DefaultMMDBMaxAge, cfg.MMDB.MaxAge.Duration)
	})

	t.Run("preserves explicit max_age", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			MMDB: &MMDBConfig{
				Enabled:      true,
				DatabasePath: "/tmp/test.mmdb",
				MaxAge:       Duration{Duration: 24 * time.Hour},
			},
		}

		cfg.Normalize()

		assert.Equal(t, 24*time.Hour, cfg.MMDB.MaxAge.Duration)
	})

	t.Run("no-op when mmdb is nil", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{}

		require.NotPanics(t, func() {
			cfg.Normalize()
		})
	})
}

func TestConfig_SingboxCLIEnabled(t *testing.T) {
	t.Parallel()

	// Проверяем, что nil SingboxCLI возвращает false.
	t.Run("nil returns false", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{}

		// act & assert
		assert.False(t, cfg.SingboxCLIEnabled())
	})

	// Проверяем, что SingboxCLI.Enabled=false возвращает false.
	t.Run("disabled returns false", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{SingboxCLI: &SingboxCLIConfig{Enabled: false}}

		// act & assert
		assert.False(t, cfg.SingboxCLIEnabled())
	})

	// Проверяем, что SingboxCLI.Enabled=true возвращает true.
	t.Run("enabled returns true", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{SingboxCLI: &SingboxCLIConfig{Enabled: true}}

		// act & assert
		assert.True(t, cfg.SingboxCLIEnabled())
	})
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	// Проверяем успешный парсинг Go duration string.
	t.Run("parses valid duration", func(t *testing.T) {
		t.Parallel()

		// arrange
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "1h30m"}

		// act
		var d Duration
		err := d.UnmarshalYAML(node)

		// assert
		require.NoError(t, err)
		assert.Equal(t, 90*time.Minute, d.Duration)
	})

	// Проверяем ошибку при невалидной строке.
	t.Run("error on invalid string", func(t *testing.T) {
		t.Parallel()

		// arrange
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "not-a-duration"}

		// act
		var d Duration
		err := d.UnmarshalYAML(node)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse duration")
	})
}

func TestDuration_MarshalYAML(t *testing.T) {
	t.Parallel()

	// Проверяем сериализацию в Go duration string.
	t.Run("returns duration string", func(t *testing.T) {
		t.Parallel()

		// arrange
		d := Duration{Duration: 5 * time.Minute}

		// act
		got, err := d.MarshalYAML()

		// assert
		require.NoError(t, err)
		assert.Equal(t, "5m0s", got)
	})
}

func TestLoad_SingboxCLIConfig(t *testing.T) {
	t.Parallel()

	// Проверяем загрузку секции singbox_cli.
	t.Run("loads singbox_cli section", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
cache_path: "/tmp/cache.json"
singbox_cli:
  enabled: true
  cli_path: "/usr/local/bin/sing-box"
sections:
  - name: "Europe"
    countries: ["Germany"]
    sources:
      - type: subscription
        urls:
          - "https://example.com/subscription"
`)
		require.NoError(t, os.WriteFile(path, data, 0o644))

		// act
		cfg, err := Load(path)

		// assert
		require.NoError(t, err)
		require.NotNil(t, cfg.SingboxCLI)
		assert.True(t, cfg.SingboxCLIEnabled())
		assert.Equal(t, "/usr/local/bin/sing-box", cfg.SingboxCLI.CLIPath)
	})
}
