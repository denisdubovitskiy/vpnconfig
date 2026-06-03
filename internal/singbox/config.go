package singbox

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config представляет конфигурацию sing-box.
type Config struct {
	Log          json.RawMessage `json:"log,omitempty"`
	DNS          json.RawMessage `json:"dns,omitempty"`
	NTP          json.RawMessage `json:"ntp,omitempty"`
	Certificate  json.RawMessage `json:"certificate,omitempty"`
	Endpoints    json.RawMessage `json:"endpoints,omitempty"`
	Inbounds     json.RawMessage `json:"inbounds,omitempty"`
	Outbounds    []Outbound      `json:"outbounds"`
	Route        json.RawMessage `json:"route,omitempty"`
	Services     json.RawMessage `json:"services,omitempty"`
	Experimental json.RawMessage `json:"experimental,omitempty"`
}

// Outbound представляет outbound конфигурацию sing-box.
// Используется map[string]any для сохранения всех полей при маршалинге.
type Outbound map[string]any

// Tag возвращает тег outbound.
func (o Outbound) Tag() string {
	if tag, ok := o["tag"].(string); ok {
		return tag
	}
	return ""
}

// SetTag устанавливает тег outbound.
func (o Outbound) SetTag(tag string) {
	o["tag"] = tag
}

// Type возвращает тип outbound.
func (o Outbound) Type() string {
	if t, ok := o["type"].(string); ok {
		return t
	}
	return ""
}

// LoadConfig загружает конфигурацию sing-box из JSON-файла.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read singbox config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse singbox config: %w", err)
	}

	return &cfg, nil
}

// SaveConfig сохраняет конфигурацию sing-box в JSON-файл.
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal singbox config: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write singbox config: %w", err)
	}

	return nil
}

// RemoveSectionOutbounds удаляет все outbounds указанной секции.
// Секция определяется по префиксу тега ("{section}-").
// Ruleset outbounds (type: local, remote) не удаляются.
func (cfg *Config) RemoveSectionOutbounds(section string) {
	var filtered []Outbound
	prefix := section + "-"

	for _, o := range cfg.Outbounds {
		if !strings.HasPrefix(o.Tag(), prefix) || isRulesetOutbound(o) {
			filtered = append(filtered, o)
		}
	}

	cfg.Outbounds = filtered
}

func isRulesetOutbound(o Outbound) bool {
	t := o.Type()
	return t == "local" || t == "remote"
}

// AddOutbounds добавляет outbounds в конец списка.
func (cfg *Config) AddOutbounds(outbounds []Outbound) {
	cfg.Outbounds = append(cfg.Outbounds, outbounds...)
}

// NewURLTestOutbound создаёт urltest outbound.
func NewURLTestOutbound(tag string, outbounds []string, testURL string, interval string, tolerance int) Outbound {
	return Outbound{
		"type":      "urltest",
		"tag":       tag,
		"outbounds": outbounds,
		"url":       testURL,
		"interval":  interval,
		"tolerance": tolerance,
	}
}

// NewSelectorOutbound создаёт selector outbound.
func NewSelectorOutbound(tag string, outbounds []string, defaultOutbound string) Outbound {
	return Outbound{
		"type":      "selector",
		"tag":       tag,
		"outbounds": outbounds,
		"default":   defaultOutbound,
	}
}

// ConvertFromSingBoxOutbound конвертирует outbound из vpnurl пакета в Outbound.
func ConvertFromSingBoxOutbound(v any) (Outbound, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal outbound: %w", err)
	}

	var outbound Outbound
	if err := json.Unmarshal(data, &outbound); err != nil {
		return nil, fmt.Errorf("unmarshal outbound: %w", err)
	}

	return outbound, nil
}

// GenerateSectionOutbounds генерирует outbounds для секции.
// Создаёт N прокси outbounds, urltest и selector.
func GenerateSectionOutbounds(
	section string,
	proxies []Outbound,
	testURL string,
	interval string,
	tolerance int,
) []Outbound {
	var result []Outbound

	// Теги прокси: section-1-out, section-2-out, ...
	var proxyTags []string
	for i, proxy := range proxies {
		tag := fmt.Sprintf("%s-%d-out", section, i+1)
		proxy.SetTag(tag)
		result = append(result, proxy)
		proxyTags = append(proxyTags, tag)
	}

	// Urltest: section-urltest-out
	urltestTag := section + "-urltest-out"
	urltest := NewURLTestOutbound(urltestTag, proxyTags, testURL, interval, tolerance)
	result = append(result, urltest)

	// Selector: section-out
	selectorTag := section + "-out"
	selectorOutbounds := append(proxyTags, urltestTag)
	selector := NewSelectorOutbound(selectorTag, selectorOutbounds, urltestTag)
	result = append(result, selector)

	return result
}

// CreateBackup создаёт backup конфигурации sing-box с timestamp в имени файла.
func CreateBackup(configPath string) (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.backup_%s", configPath, timestamp)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config for backup: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write backup file: %w", err)
	}

	return backupPath, nil
}
