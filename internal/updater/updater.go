package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
	"github.com/denisdubovitskiy/vpnconfig/internal/profile"
	"github.com/denisdubovitskiy/vpnconfig/internal/resolver"
	"github.com/denisdubovitskiy/vpnconfig/internal/singbox"
	"github.com/denisdubovitskiy/vpnconfig/internal/vpnurl"
)

// LinkFetcher получает список VPN-ссылок. Алиас для profile.LinkFetcher,
// чтобы updater мог принимать мапу с любыми реализациями профилей без
// дополнительных адаптеров.
type LinkFetcher = profile.LinkFetcher

// DNSResolver резолвит доменное имя в список IP-адресов. Алиас для
// resolver.IPResolver — единый интерфейс для всех DNS-резолверов.
type DNSResolver = resolver.IPResolver

// GeoIPService определяет страну по IP-адресу.
type GeoIPService interface {
	CountryName(ctx context.Context, ip string) (string, error)
}

// VPNParser парсит VPN URL в outbound конфигурацию.
type VPNParser interface {
	Parse(vpnURL string) (vpnurl.SingBoxOutbound, error)
}

// ConfigStore управляет конфигурацией sing-box.
type ConfigStore interface {
	LoadConfig(path string) (*singbox.Config, error)
	SaveConfig(path string, cfg *singbox.Config) error
	CreateBackup(configPath string) (string, error)
}

// ConfigValidator проверяет валидность конфигурации sing-box.
type ConfigValidator interface {
	CheckConfig(ctx context.Context, configPath string) error
}

// LinkChecker проверяет работоспособность VPN-ссылки через локальный sing-box.
type LinkChecker interface {
	CheckLink(ctx context.Context, vlessLink string) error
}

// Updater обновляет sing-box конфигурацию на основе VPN-ссылок.
type Updater struct {
	fetchers    map[config.SourceType]LinkFetcher
	dns         DNSResolver
	geoIP       GeoIPService
	parser      VPNParser
	configStore ConfigStore
	validator   ConfigValidator
	checker     LinkChecker
}

// NewUpdater создаёт новый Updater. checker может быть nil — тогда проверка
// VPN-ссылок не выполняется.
func NewUpdater(
	fetchers map[config.SourceType]LinkFetcher,
	dns DNSResolver,
	geoIP GeoIPService,
	parser VPNParser,
	configStore ConfigStore,
	validator ConfigValidator,
	checker LinkChecker,
) *Updater {
	return &Updater{
		fetchers:    fetchers,
		dns:         dns,
		geoIP:       geoIP,
		parser:      parser,
		configStore: configStore,
		validator:   validator,
		checker:     checker,
	}
}

// Result содержит результаты обновления.
type Result struct {
	LinksFetched    int
	URLsParsed      int
	CountriesFound  map[string]int
	SectionsUpdated []string
	BackupPath      string
	Changed         bool
}

// Run выполняет полный цикл обновления конфигурации.
// Логгер берётся из ctx через logger.FromContext; в ctx он должен быть
// помещён вызывающим кодом (см. logger.IntoContext).
func (u *Updater) Run(ctx context.Context, cfg *config.Config) (*Result, error) {
	log := logger.FromContext(ctx)

	log.Info("starting update cycle",
		"cache_path", cfg.CachePath,
		"singbox_config", cfg.SingboxConfig,
		"sections_count", len(cfg.Sections),
	)

	countryURLs := make(map[string][]string)
	parsedOutbounds := make(map[string]singbox.Outbound)
	totalLinks := 0
	skipped := 0

	for _, section := range cfg.Sections {
		for _, source := range section.Sources {
			fetcher, ok := u.fetchers[source.Type]
			if !ok {
				log.Warn("unknown source type, skipping",
					"section", section.Name,
					"type", string(source.Type),
				)
				continue
			}

			for _, sourceURL := range source.URLs {
				log.Info("fetching links from source",
					"section", section.Name,
					"type", string(source.Type),
					"url", sourceURL,
				)

				links, err := fetcher.FetchLinks(ctx, sourceURL)
				if err != nil {
					log.Warn("failed to fetch links from source",
						"section", section.Name,
						"type", string(source.Type),
						"url", sourceURL,
						"reason", err.Error(),
					)
					continue
				}

				totalLinks += len(links)

				for _, l := range links {
					ip, err := parseIPFromVpnURL(ctx, u.dns, l)
					if err != nil {
						log.Warn("skipping url: failed to extract IP",
							"url", truncateURL(l),
							"reason", err.Error(),
						)
						skipped++
						continue
					}

					country, err := u.geoIP.CountryName(ctx, ip)
					if err != nil {
						log.Warn("skipping url: geoip lookup failed",
							"url", truncateURL(l),
							"ip", ip,
							"reason", err.Error(),
						)
						skipped++
						continue
					}

					parsed, err := u.parser.Parse(l)
					if err != nil {
						log.Warn("skipping url: parser failed",
							"url", truncateURL(l),
							"ip", ip,
							"country", country,
							"reason", err.Error(),
						)
						skipped++
						continue
					}

					outbound, err := singbox.ConvertFromSingBoxOutbound(parsed.ToOutbound())
					if err != nil {
						log.Warn("skipping url: conversion failed",
							"url", truncateURL(l),
							"ip", ip,
							"country", country,
							"reason", err.Error(),
						)
						skipped++
						continue
					}

					log.Info("parsed url successfully",
						"ip", ip,
						"country", country,
						"type", parsed.Type(),
					)

					if u.checker != nil {
						if err := u.checker.CheckLink(ctx, l); err != nil {
							log.Warn("skipping url: link check failed",
								"url", truncateURL(l),
								"ip", ip,
								"country", country,
								"reason", err.Error(),
							)
							skipped++
							continue
						}
						log.Info("link check passed", "url", truncateURL(l))
					}

					countryURLs[country] = append(countryURLs[country], l)
					parsedOutbounds[l] = outbound
				}
			}
		}
	}

	if skipped > 0 {
		log.Info("url processing summary",
			"total", totalLinks,
			"successful", len(parsedOutbounds),
			"skipped", skipped,
		)
	}

	// Одна и та же ссылка может встретиться в нескольких секциях/источниках,
	// поэтому дедуплицируем countryURLs — иначе outbounds в sing-box конфиге
	// будут дублироваться.
	deduplicateCountryURLs(countryURLs)

	u.logCountriesSummary(ctx, countryURLs)

	singboxCfg, err := u.configStore.LoadConfig(cfg.SingboxConfig)
	if err != nil {
		return nil, fmt.Errorf("load singbox config: %w", err)
	}

	oldOutbounds, err := singboxCfg.CloneOutbounds()
	if err != nil {
		return nil, fmt.Errorf("clone outbounds: %w", err)
	}

	log.Info("loaded sing-box config",
		"path", cfg.SingboxConfig,
		"outbounds_count", len(singboxCfg.Outbounds),
	)

	var sectionsUpdated []string
	for _, section := range cfg.Sections {
		log.Info("processing section",
			"section", section.Name,
			"allowed_countries", section.Countries,
		)

		sectionOutbounds := u.buildSectionOutbounds(section, countryURLs, parsedOutbounds)
		if len(sectionOutbounds) == 0 {
			log.Warn("section has no matching outbounds, skipping",
				"section", section.Name,
				"reason", "no servers from allowed countries were found",
			)
			continue
		}

		log.Info("building section outbounds",
			"section", section.Name,
			"proxy_count", len(sectionOutbounds),
		)

		removed := singboxCfg.RemoveSectionOutbounds(section.Name)
		if removed > 0 {
			log.Info("removed old section outbounds",
				"section", section.Name,
				"removed_count", removed,
			)
		}

		urltestURL, urltestInterval, urltestTolerance := u.resolveURLTestSettings(cfg, section)
		newOutbounds := singbox.GenerateSectionOutbounds(
			section.Name,
			sectionOutbounds,
			urltestURL,
			urltestInterval,
			urltestTolerance,
		)

		singboxCfg.AddOutbounds(newOutbounds)
		sectionsUpdated = append(sectionsUpdated, section.Name)

		log.Info("section updated",
			"section", section.Name,
			"added_proxies", len(sectionOutbounds),
			"total_outbounds", len(singboxCfg.Outbounds),
		)
	}

	countriesFound := make(map[string]int)
	for country, urls := range countryURLs {
		countriesFound[country] = len(urls)
	}

	// Сравниваем outbounds через JSON-представление, а не через reflect.DeepEqual.
	// reflect.DeepEqual чувствителен к Go-типам значений: int(50) != float64(50),
	// хотя JSON-сериализация идентична. После клонирования через CloneOutbounds
	// (json.Marshal + json.Unmarshal) все числа становятся float64, а
	// сгенерированные через NewURLTestOutbound outbounds содержат int в поле
	// tolerance. Без JSON-сравнения это приводит к ложноположительным
	// "changes detected" при повторных запусках с теми же данными.
	equal, err := outboundsEqual(oldOutbounds, singboxCfg.Outbounds)
	if err != nil {
		return nil, fmt.Errorf("compare outbounds: %w", err)
	}
	if equal {
		log.Info("no changes detected in sing-box config, skipping save",
			"reason", "outbounds are identical to previous state",
		)
		return &Result{
			LinksFetched:    totalLinks,
			URLsParsed:      len(parsedOutbounds),
			CountriesFound:  countriesFound,
			SectionsUpdated: sectionsUpdated,
			Changed:         false,
		}, nil
	}

	log.Info("changes detected in sing-box config",
		"old_outbound_count", len(oldOutbounds),
		"new_outbound_count", len(singboxCfg.Outbounds),
		"sections_updated", sectionsUpdated,
	)

	if err := u.cleanupCacheIfNeeded(ctx, cfg.CachePath, cfg.MaxCacheSizeBytes); err != nil {
		log.Warn("cache cleanup failed", "error", err.Error())
	}

	backupPath, err := u.createBackup(cfg.SingboxConfig, cfg.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("create backup: %w", err)
	}
	log.Info("backup created", "path", backupPath)

	if cfg.MaxBackups > 0 {
		if err := u.cleanupOldBackups(ctx, cfg.BackupDir, cfg.MaxBackups); err != nil {
			log.Warn("old backup cleanup failed", "error", err.Error())
		}
	}

	if err := u.saveConfigWithValidation(ctx, cfg.SingboxConfig, singboxCfg); err != nil {
		return nil, fmt.Errorf("save singbox config: %w", err)
	}

	log.Info("sing-box config saved", "path", cfg.SingboxConfig)

	return &Result{
		LinksFetched:    totalLinks,
		URLsParsed:      len(parsedOutbounds),
		CountriesFound:  countriesFound,
		SectionsUpdated: sectionsUpdated,
		BackupPath:      backupPath,
		Changed:         true,
	}, nil
}

// deduplicateCountryURLs удаляет повторяющиеся URL внутри каждой страны.
// Сохраняет порядок первого вхождения.
func deduplicateCountryURLs(countryURLs map[string][]string) {
	for country, urls := range countryURLs {
		seen := make(map[string]struct{}, len(urls))
		unique := make([]string, 0, len(urls))
		for _, u := range urls {
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			unique = append(unique, u)
		}
		countryURLs[country] = unique
	}
}

func (u *Updater) logCountriesSummary(ctx context.Context, countryURLs map[string][]string) {
	log := logger.FromContext(ctx)

	if len(countryURLs) == 0 {
		log.Warn("no valid countries found in any URLs")
		return
	}

	countries := make([]string, 0, len(countryURLs))
	for c := range countryURLs {
		countries = append(countries, c)
	}
	slices.Sort(countries)

	for _, country := range countries {
		log.Info("country summary",
			"country", country,
			"urls", len(countryURLs[country]),
		)
	}
}

func (u *Updater) buildSectionOutbounds(
	section config.Section,
	countryURLs map[string][]string,
	parsedOutbounds map[string]singbox.Outbound,
) []singbox.Outbound {
	countries := make([]string, 0, len(countryURLs))
	for country := range countryURLs {
		countries = append(countries, country)
	}
	slices.Sort(countries)

	var result []singbox.Outbound
	for _, country := range countries {
		if !slices.Contains(section.Countries, country) {
			continue
		}
		for _, url := range countryURLs[country] {
			if ob, ok := parsedOutbounds[url]; ok {
				result = append(result, ob)
			}
		}
	}
	return result
}

func (u *Updater) resolveURLTestSettings(
	cfg *config.Config,
	section config.Section,
) (string, string, int) {
	urltestURL := cfg.URLTestDefaults.URL
	urltestInterval := cfg.URLTestDefaults.Interval
	urltestTolerance := cfg.URLTestDefaults.Tolerance

	if section.URLTest != nil {
		if section.URLTest.URL != "" {
			urltestURL = section.URLTest.URL
		}
		if section.URLTest.Interval != "" {
			urltestInterval = section.URLTest.Interval
		}
		if section.URLTest.Tolerance != 0 {
			urltestTolerance = section.URLTest.Tolerance
		}
	}

	return urltestURL, urltestInterval, urltestTolerance
}

func (u *Updater) cleanupCacheIfNeeded(ctx context.Context, cachePath string, maxSizeBytes int64) error {
	if maxSizeBytes <= 0 || cachePath == "" {
		return nil
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat cache file: %w", err)
	}

	if info.Size() > maxSizeBytes {
		if err := os.WriteFile(cachePath, []byte("{}"), 0o644); err != nil {
			return fmt.Errorf("clear cache file: %w", err)
		}
		logger.FromContext(ctx).Info("cache cleared due to size limit",
			"path", cachePath,
			"size_bytes", info.Size(),
			"limit_bytes", maxSizeBytes,
		)
	}

	return nil
}

func (u *Updater) createBackup(configPath, backupDir string) (string, error) {
	if backupDir == "" {
		return u.configStore.CreateBackup(configPath)
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config for backup: %w", err)
	}

	filename := filepath.Base(configPath) + ".backup_" + time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, filename)

	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write backup file: %w", err)
	}

	return backupPath, nil
}

func (u *Updater) cleanupOldBackups(ctx context.Context, backupDir string, maxBackups int) error {
	if backupDir == "" || maxBackups <= 0 {
		return nil
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read backup dir: %w", err)
	}

	var backups []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "singbox.json.backup_") {
			backups = append(backups, entry)
		}
	}

	if len(backups) <= maxBackups {
		return nil
	}

	sort.Slice(backups, func(i, j int) bool {
		infoI, _ := backups[i].Info()
		infoJ, _ := backups[j].Info()
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	log := logger.FromContext(ctx)
	toDelete := len(backups) - maxBackups
	for i := 0; i < toDelete; i++ {
		path := filepath.Join(backupDir, backups[i].Name())
		if err := os.Remove(path); err != nil {
			log.Warn("failed to remove old backup", "path", path, "error", err.Error())
		} else {
			log.Info("removed old backup", "path", path)
		}
	}

	return nil
}

// parseIPFromVpnURL извлекает IP-адрес из VPN-ссылки. Если хост ссылки —
// доменное имя, резолвит его через DNS и возвращает первый IP.
func parseIPFromVpnURL(ctx context.Context, dns DNSResolver, vpnURL string) (string, error) {
	if !strings.Contains(vpnURL, "://") {
		return "", fmt.Errorf("invalid url: no scheme")
	}

	scheme := strings.Split(vpnURL, "://")[0]

	if scheme == "vmess" {
		return "", fmt.Errorf("%s is not supported", scheme)
	}

	u, err := url.Parse(vpnURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("empty host")
	}

	if ip := net.ParseIP(host); ip != nil {
		return host, nil
	}

	ips, err := dns.LookupIP(ctx, "ip", host)
	if err != nil {
		return "", fmt.Errorf("resolve domain %s: %w", host, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no addresses for domain %s", host)
	}
	return ips[0].String(), nil
}

// truncateURL обрезает URL для логирования, оставляя только схему и хост.
func truncateURL(vpnURL string) string {
	u, err := url.Parse(vpnURL)
	if err != nil {
		return vpnURL
	}
	return u.Scheme + "://" + u.Host
}

// saveConfigWithValidation сохраняет конфигурацию с опциональной проверкой через CLI.
// Если validator == nil, сохраняет напрямую. Если validator != nil, сохраняет во
// временный файл, проверяет, и только потом заменяет оригинальный файл.
func (u *Updater) saveConfigWithValidation(
	ctx context.Context,
	configPath string,
	cfg *singbox.Config,
) error {
	if u.validator == nil {
		return u.configStore.SaveConfig(configPath, cfg)
	}

	tmpPath := configPath + ".tmp"
	if err := u.configStore.SaveConfig(tmpPath, cfg); err != nil {
		return fmt.Errorf("save temp config: %w", err)
	}

	if err := u.validator.CheckConfig(ctx, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("config validation failed: %w", err)
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config file: %w", err)
	}

	return nil
}

// outboundsEqual сравнивает два списка outbounds через JSON-представление.
// Нужно вместо reflect.DeepEqual, потому что после CloneOutbounds (json
// round-trip) все числа становятся float64, а свежесгенерированные
// outbounds могут содержать int (например, tolerance в NewURLTestOutbound).
// reflect.DeepEqual считает int(50) != float64(50), хотя данные идентичны.
func outboundsEqual(a, b []singbox.Outbound) (bool, error) {
	aJSON, err := json.Marshal(a)
	if err != nil {
		return false, fmt.Errorf("marshal outbounds a: %w", err)
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		return false, fmt.Errorf("marshal outbounds b: %w", err)
	}
	return bytes.Equal(aJSON, bJSON), nil
}
