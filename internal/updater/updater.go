package updater

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/singbox"
	"github.com/denisdubovitskiy/vpnconfig/internal/vpnurl"
)

// LinkFetcher получает список VPN-ссылок.
type LinkFetcher interface {
	FetchLinks(ctx context.Context, url string) ([]string, error)
}

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

// Updater обновляет sing-box конфигурацию на основе VPN-ссылок.
type Updater struct {
	fetcher     LinkFetcher
	geoIP       GeoIPService
	parser      VPNParser
	configStore ConfigStore
	logger      *slog.Logger
}

// NewUpdater создаёт новый Updater.
func NewUpdater(
	fetcher LinkFetcher,
	geoIP GeoIPService,
	parser VPNParser,
	configStore ConfigStore,
	logger *slog.Logger,
) *Updater {
	return &Updater{
		fetcher:     fetcher,
		geoIP:       geoIP,
		parser:      parser,
		configStore: configStore,
		logger:      logger,
	}
}

// Result содержит результаты обновления.
type Result struct {
	LinksFetched    int
	URLsParsed      int
	CountriesFound  map[string]int
	SectionsUpdated []string
	BackupPath      string
}

// Run выполняет полный цикл обновления конфигурации.
func (u *Updater) Run(ctx context.Context, cfg *config.Config) (*Result, error) {
	links, err := u.fetcher.FetchLinks(ctx, cfg.HappURL)
	if err != nil {
		return nil, fmt.Errorf("fetch links: %w", err)
	}

	u.logger.Info("parsing urls and looking up countries", "count", len(links))

	countryURLs := make(map[string][]string)
	parsedOutbounds := make(map[string]singbox.Outbound)

	for _, l := range links {
		ip, err := parseIPFromVpnURL(l)
		if err != nil {
			u.logger.Error("skipping url due to an error", "err", err.Error())
			continue
		}

		country, err := u.geoIP.CountryName(ctx, ip)
		if err != nil {
			u.logger.Error("skipping url due to country parse error", "err", err.Error())
			continue
		}

		u.logger.Info("country name parsed", "country_name", country, "ip", ip)

		parsed, err := u.parser.Parse(l)
		if err != nil {
			u.logger.Error("skipping url due to parse error", "err", err.Error())
			continue
		}

		outbound, err := singbox.ConvertFromSingBoxOutbound(parsed.ToOutbound())
		if err != nil {
			u.logger.Error("skipping url due to convert error", "err", err.Error())
			continue
		}

		countryURLs[country] = append(countryURLs[country], l)
		parsedOutbounds[l] = outbound
	}

	singboxCfg, err := u.configStore.LoadConfig(cfg.SingboxConfig)
	if err != nil {
		return nil, fmt.Errorf("load singbox config: %w", err)
	}

	if err := u.cleanupCacheIfNeeded(cfg.CachePath, cfg.MaxCacheSizeBytes); err != nil {
		u.logger.Warn("failed to cleanup cache", "error", err.Error())
	}

	backupPath, err := u.createBackup(cfg.SingboxConfig, cfg.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("create backup: %w", err)
	}
	u.logger.Info("backup created", "path", backupPath)

	if cfg.MaxBackups > 0 {
		if err := u.cleanupOldBackups(cfg.BackupDir, cfg.MaxBackups); err != nil {
			u.logger.Warn("failed to cleanup old backups", "error", err.Error())
		}
	}

	var sectionsUpdated []string
	for _, section := range cfg.Sections {
		u.logger.Info("updating section", "section", section.Name)

		sectionOutbounds := u.buildSectionOutbounds(section, countryURLs, parsedOutbounds)
		if len(sectionOutbounds) == 0 {
			u.logger.Warn("no outbounds found for section, skipping", "section", section.Name)
			continue
		}

		u.logger.Info("found outbounds for section", "section", section.Name, "count", len(sectionOutbounds))

		singboxCfg.RemoveSectionOutbounds(section.Name)

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
	}

	if err := u.configStore.SaveConfig(cfg.SingboxConfig, singboxCfg); err != nil {
		return nil, fmt.Errorf("save singbox config: %w", err)
	}

	u.logger.Info("singbox config updated successfully")

	countriesFound := make(map[string]int)
	for country, urls := range countryURLs {
		countriesFound[country] = len(urls)
	}

	return &Result{
		LinksFetched:    len(links),
		URLsParsed:      len(parsedOutbounds),
		CountriesFound:  countriesFound,
		SectionsUpdated: sectionsUpdated,
		BackupPath:      backupPath,
	}, nil
}

func (u *Updater) buildSectionOutbounds(
	section config.Section,
	countryURLs map[string][]string,
	parsedOutbounds map[string]singbox.Outbound,
) []singbox.Outbound {
	var result []singbox.Outbound
	for country, urls := range countryURLs {
		if !slices.Contains(section.Countries, country) {
			continue
		}
		for _, url := range urls {
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

func (u *Updater) cleanupCacheIfNeeded(cachePath string, maxSizeBytes int64) error {
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
		u.logger.Info("cache cleared due to size limit", "path", cachePath, "size", info.Size(), "limit", maxSizeBytes)
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

func (u *Updater) cleanupOldBackups(backupDir string, maxBackups int) error {
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

	toDelete := len(backups) - maxBackups
	for i := 0; i < toDelete; i++ {
		path := filepath.Join(backupDir, backups[i].Name())
		if err := os.Remove(path); err != nil {
			u.logger.Warn("failed to remove old backup", "path", path, "error", err.Error())
		} else {
			u.logger.Info("removed old backup", "path", path)
		}
	}

	return nil
}

// parseIPFromVpnURL извлекает IP-адрес из VPN-ссылки.
func parseIPFromVpnURL(vpnURL string) (string, error) {
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

	if net.ParseIP(host) == nil {
		return "", fmt.Errorf("host is not a valid IP: %s", host)
	}

	return host, nil
}
