package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
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

// Section представляет секцию конфигурации с фильтрацией по странам.
type Section struct {
	// Name — имя секции.
	Name string `yaml:"name"`
	// Countries — список стран для фильтрации.
	Countries []string `yaml:"countries"`
	// URLTest — настройки urltest для этой секции (опционально).
	URLTest *URLTestDefaults `yaml:"urltest,omitempty"`
}

// Config представляет конфигурацию приложения.
type Config struct {
	// HappURL — URL для получения списка VPN-ссылок.
	HappURL string `yaml:"happ_url"`
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
	// URLTestDefaults — настройки по умолчанию для urltest.
	URLTestDefaults URLTestDefaults `yaml:"urltest_defaults"`
	// Sections — список секций для фильтрации.
	Sections []Section `yaml:"sections"`
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

	return &cfg, nil
}
