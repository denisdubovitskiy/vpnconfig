package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	stdhttp "net/http"
	"os"

	"github.com/denisdubovitskiy/vpnconfig/internal/command"
	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/http"
	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv/mmdb"
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

func main() {
	conf, err := config.Load("config.yaml")
	if err != nil {
		slog.Error("failed to load config", "error", err.Error())
		os.Exit(1)
		return
	}

	log, logCloser, err := logger.New(conf.LogDir)
	if err != nil {
		slog.Error("failed to initialize logger", "error", err.Error())
		os.Exit(1)
		return
	}
	//nolint:errcheck // defer Close — стандартная идиома.
	defer logCloser.Close()

	fail := func(code int) {
		_ = logCloser.Close()
		os.Exit(code)
	}

	log.Info("starting vpnconfig updater",
		"version", "dev",
		"config_path", "config.yaml",
		"singbox_config", conf.SingboxConfig,
		"sections_count", len(conf.Sections),
	)

	for i, s := range conf.Sections {
		log.Info("configured section",
			"index", i,
			"name", s.Name,
			"countries", fmt.Sprintf("%v", s.Countries),
		)
	}

	ctx := logger.IntoContext(context.Background(), log)
	httpClient := http.NewClient()

	geo, mmdbCloser, err := setupGeoService(ctx, conf, httpClient)
	if err != nil {
		log.Error("failed to create geo service", "error", err.Error())
		fail(1)
	}
	if mmdbCloser != nil {
		defer func() {
			if err := mmdbCloser.Close(); err != nil {
				log.Warn("failed to close mmdb provider", "error", err.Error())
			}
		}()
	}

	fetchers := map[config.SourceType]profile.LinkFetcher{
		config.SourceTypeSubscription: subscription.NewClient(httpClient),
		config.SourceTypePlaintext:    plaintext.NewClient(httpClient),
	}

	dnsResolver, err := newDNSResolver(ctx, conf)
	if err != nil {
		log.Error("failed to create dns resolver", "error", err.Error())
		fail(1)
	}

	u := updater.NewUpdater(
		fetchers,
		dnsResolver,
		geo,
		vpnurl.NewParser(),
		&singbox.Store{},
		newValidator(ctx, conf),
	)

	result, err := u.Run(ctx, conf)
	if err != nil {
		log.Error("update failed", "error", err.Error())
		fail(1)
	}

	log.Info("update completed",
		"changed", result.Changed,
		"links_fetched", result.LinksFetched,
		"urls_parsed", result.URLsParsed,
		"sections_updated", result.SectionsUpdated,
		"backup_path", result.BackupPath,
	)

	if !result.Changed {
		log.Info("no changes detected — sing-box config was not modified")
	} else {
		log.Info("sing-box config updated successfully",
			"sections", fmt.Sprintf("%v", result.SectionsUpdated),
		)
	}
}

func setupGeoService(
	ctx context.Context,
	cfg *config.Config,
	httpClient *stdhttp.Client,
) (
	*ipserv.CachedIPLookup,
	*mmdb.Provider,
	error,
) {
	log := logger.FromContext(ctx)

	geoProviderNames := cfg.EffectiveGeoProviders()
	geoProviders := make([]ipserv.IPLookup, 0, len(geoProviderNames)+1)

	var mmdbProvider *mmdb.Provider
	if cfg.MMDBEnabled() {
		mp, err := mmdb.New(ctx, cfg.MMDB, httpClient)
		if err != nil {
			return nil, nil, err
		}
		mmdbProvider = mp
		geoProviders = append(geoProviders, mmdbProvider)
		log.Info("mmdb provider enabled",
			"database_path", cfg.MMDB.DatabasePath,
		)
	}

	for _, name := range geoProviderNames {
		provider, err := providers.NewByName(name, httpClient)
		if err != nil {
			return nil, nil, err
		}
		geoProviders = append(geoProviders, provider)
	}

	log.Info("geo providers configured",
		"count", len(geoProviders),
		"providers", geoProviderNames,
	)

	fallback := ipserv.NewFallback(geoProviders)
	cache := ipserv.NewFileCache(cfg.CachePath)
	service := ipserv.NewCachedIPLookup(fallback, cache)

	return service, mmdbProvider, nil
}

func newValidator(ctx context.Context, cfg *config.Config) updater.ConfigValidator {
	log := logger.FromContext(ctx)

	if cfg.SingboxCLIEnabled() {
		log.Info("sing-box CLI validation enabled",
			"cli_path", cfg.SingboxCLI.CLIPath,
		)
		return singboxcli.NewCLIChecker(cfg.SingboxCLI.CLIPath, command.NewDefaultExecutor())
	}

	return singboxcli.NewNullChecker()
}

func newDNSResolver(ctx context.Context, cfg *config.Config) (resolver.IPResolver, error) {
	log := logger.FromContext(ctx)

	if len(cfg.DNSResolvers) == 0 {
		log.Info("dns resolver: using system default (net.DefaultResolver)")
		return net.DefaultResolver, nil
	}

	opts, err := resolver.OptionsFromURLs(cfg.DNSResolvers)
	if err != nil {
		return nil, fmt.Errorf("parse dns_resolvers: %w", err)
	}

	log.Info("dns resolver: custom chain configured",
		"count", len(cfg.DNSResolvers),
		"urls", cfg.DNSResolvers,
	)

	return resolver.New(opts...), nil
}
