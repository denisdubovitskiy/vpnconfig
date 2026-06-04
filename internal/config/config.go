package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv/providers"
)

// URLTestDefaults представляет настройки по умолчанию для urltest outbound.
type URLTestDefaults struct {
	// URL — адрес для проверки задержки.
	URL string `yaml:"url"`
	// Interval — интервал проверки.
	Interval string `yaml:"interval"`
	// Tolerance — допуск задержки в миллисекундах.
	Tolerance int `yaml:"tolerance"`
}

// DefaultMMDBDownloadURL — URL по умолчанию для скачивания базы GeoLite2-Country.
const DefaultMMDBDownloadURL = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb"

// DefaultMMDBMaxAge — максимально допустимый возраст MMDB-файла по умолчанию.
// Применяется в Normalize, если в YAML max_age не указан.
const DefaultMMDBMaxAge = 168 * time.Hour

// SingboxCLIConfig описывает настройки CLI-утилиты sing-box для проверки конфигурации.
type SingboxCLIConfig struct {
	// Enabled — флаг активации проверки конфига через CLI.
	Enabled bool `yaml:"enabled"`
	// CLIPath — путь к утилите sing-box. Если пустая строка — используется "sing-box" из PATH.
	CLIPath string `yaml:"cli_path,omitempty"`
}

// MMDBConfig описывает настройки локального MMDB-провайдера определения страны
// по IP на основе MaxMind GeoLite2-Country. Провайдер опционален: при
// MMDB == nil или MMDB.Enabled == false используются только HTTP-провайдеры.
type MMDBConfig struct {
	// Enabled — флаг активации MMDB-провайдера. По умолчанию false.
	Enabled bool `yaml:"enabled"`
	// DatabasePath — путь к файлу базы данных (.mmdb).
	// При отсутствии файла он будет скачан по DownloadURL.
	DatabasePath string `yaml:"database_path"`
	// DownloadURL — URL для скачивания базы. По умолчанию
	// используется DefaultMMDBDownloadURL (P3TERX/GeoLite.mmdb).
	DownloadURL string `yaml:"download_url"`
	// MaxAge — максимально допустимый возраст файла базы. Если файл
	// старше MaxAge, он перезагружается автоматически при старте.
	// По умолчанию (если поле не задано в YAML) используется
	// DefaultMMDBMaxAge (168h = 1 неделя) — применяется в Normalize.
	// Формат значения — Go duration string: "720h", "30m", "24h".
	MaxAge Duration `yaml:"max_age,omitempty"`
}

// SourceType тип источника подписки.
type SourceType string

const (
	// SourceTypeHapp — base64-encoded подписка hynet.space.
	SourceTypeHapp SourceType = "happ"
	// SourceTypePlaintext — plain text подписка (одна ссылка на строку).
	SourceTypePlaintext SourceType = "plaintext"
)

// Source описывает один источник VPN-ссылок.
type Source struct {
	// Type — тип источника (happ или plaintext).
	Type SourceType `yaml:"type"`
	// URLs — список URL для получения ссылок.
	URLs []string `yaml:"urls"`
}

// Section представляет секцию конфигурации с фильтрацией по странам.
type Section struct {
	// Name — имя секции.
	Name string `yaml:"name"`
	// Countries — список стран для фильтрации.
	Countries []string `yaml:"countries"`
	// Sources — список источников VPN-ссылок.
	Sources []Source `yaml:"sources"`
	// URLTest — настройки urltest для этой секции (опционально).
	URLTest *URLTestDefaults `yaml:"urltest,omitempty"`
}

// Config представляет конфигурацию приложения.
type Config struct {
	// CachePath — путь к файлу кеша.
	CachePath string `yaml:"cache_path"`
	// SingboxConfig — путь к файлу конфигурации sing-box.
	SingboxConfig string `yaml:"singbox_config"`
	// BackupDir — директория для сохранения бэкапов.
	BackupDir string `yaml:"backup_dir"`
	// MaxBackups — максимальное количество файлов бэкапов.
	MaxBackups int `yaml:"max_backups"`
	// MaxCacheSizeBytes — максимальный размер файла кеша в байтах.
	MaxCacheSizeBytes int64 `yaml:"max_cache_size_bytes"`
	// LogDir — директория для файлов логов. Если пустая — логи только в stderr.
	LogDir string `yaml:"log_dir"`
	// URLTestDefaults — настройки по умолчанию для urltest.
	URLTestDefaults URLTestDefaults `yaml:"urltest_defaults"`
	// Sections — список секций для фильтрации.
	Sections []Section `yaml:"sections"`
	// GeoProviders — список провайдеров для определения страны по IP.
	// Если пусто — используются все встроенные провайдеры в порядке по умолчанию.
	// Порядок элементов определяет приоритет fallback: первый — самый предпочтительный.
	GeoProviders []string `yaml:"geo_providers"`
	// MMDB — настройки локального MMDB-провайдера. nil или Enabled == false
	// означает, что провайдер не используется.
	MMDB *MMDBConfig `yaml:"mmdb,omitempty"`
	// SingboxCLI — настройки CLI-утилиты sing-box для проверки конфигурации.
	// Если nil или Enabled == false — проверка конфига не выполняется.
	SingboxCLI *SingboxCLIConfig `yaml:"singbox_cli,omitempty"`
	// DNSResolvers — список DNS-резолверов для определения IP по доменному имени.
	// Используется при парсинге VPN-ссылок, если хост — это домен.
	//
	// Поддерживаемые схемы URL:
	//   - "https://host/path" — DNS over HTTPS (путь обязателен)
	//   - "tls://host[:port]"  — DNS over TLS (порт по умолчанию 853)
	//   - "host[:port]"        — обычный DNS через UDP/TCP (порт по умолчанию 53)
	//   - "ip[:port]"          — обычный DNS по IP-адресу
	//
	// Порядок в списке определяет приоритет fallback: первый — самый
	// предпочтительный. Если список пуст — используется системный резолвер
	// (net.DefaultResolver).
	DNSResolvers []string `yaml:"dns_resolvers,omitempty"`
}

// Load читает конфигурацию из YAML-файла.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.Normalize()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Normalize заполняет дефолтные значения полей, не заданных в YAML.
// Вызывается автоматически из Load; можно вызвать вручную на тестовых фикстурах.
func (c *Config) Normalize() {
	if c.MMDB != nil && c.MMDB.MaxAge.Duration == 0 {
		c.MMDB.MaxAge.Duration = DefaultMMDBMaxAge
	}
}

// Validate проверяет корректность конфигурации после десериализации.
func (c *Config) Validate() error {
	if len(c.GeoProviders) > 0 {
		known := make(map[string]struct{}, len(providers.DefaultNames()))
		for _, name := range providers.DefaultNames() {
			known[name] = struct{}{}
		}

		for _, name := range c.GeoProviders {
			if _, ok := known[name]; !ok {
				return fmt.Errorf("unknown geo provider %q (available: %v)", name, providers.DefaultNames())
			}
		}
	}

	if c.MMDB != nil && c.MMDB.Enabled && c.MMDB.DatabasePath == "" {
		return fmt.Errorf("mmdb.database_path is required when mmdb.enabled is true")
	}

	for _, section := range c.Sections {
		if len(section.Sources) == 0 {
			return fmt.Errorf("section %q must have at least one source", section.Name)
		}
		for _, source := range section.Sources {
			if source.Type != SourceTypeHapp && source.Type != SourceTypePlaintext {
				return fmt.Errorf("section %q: unknown source type %q (available: happ, plaintext)", section.Name, source.Type)
			}
			if len(source.URLs) == 0 {
				return fmt.Errorf("section %q: source type %q must have at least one URL", section.Name, source.Type)
			}
		}
	}

	return nil
}

// EffectiveGeoProviders возвращает список провайдеров, которые нужно использовать.
// Если в конфиге GeoProviders не указан — возвращает DefaultNames.
func (c *Config) EffectiveGeoProviders() []string {
	if len(c.GeoProviders) == 0 {
		return providers.DefaultNames()
	}
	return c.GeoProviders
}

// MMDBEnabled возвращает true, если MMDB-провайдер явно активирован в конфиге
// (секция mmdb присутствует и mmdb.enabled == true).
func (c *Config) MMDBEnabled() bool {
	return c.MMDB != nil && c.MMDB.Enabled
}

// SingboxCLIEnabled возвращает true, если CLI-проверка конфига активирована.
func (c *Config) SingboxCLIEnabled() bool {
	return c.SingboxCLI != nil && c.SingboxCLI.Enabled
}

// EffectiveMMDBDownloadURL возвращает URL для скачивания MMDB-базы.
// Если в конфиге URL не задан, используется DefaultMMDBDownloadURL.
func (c *MMDBConfig) EffectiveDownloadURL() string {
	if c.DownloadURL != "" {
		return c.DownloadURL
	}
	return DefaultMMDBDownloadURL
}

// Duration — обёртка над time.Duration для парсинга из YAML в формате
// Go duration string ("24h", "30m", "1h30m"). yaml.v3 не парсит
// time.Duration напрямую.
type Duration struct {
	time.Duration
}

// UnmarshalYAML декодирует строку формата Go duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalYAML кодирует значение в Go duration string.
func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}
