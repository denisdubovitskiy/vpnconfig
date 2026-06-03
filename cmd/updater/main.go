package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/happ"
	"github.com/denisdubovitskiy/vpnconfig/internal/http"
	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
	"github.com/denisdubovitskiy/vpnconfig/internal/singbox"
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

	ctx := context.Background()

	httpClient := http.NewClient()

	happClient := happ.NewClient(httpClient)

	ipservClient := ipserv.NewClient(httpClient)
	iservCache := ipserv.NewFileCache(conf.CachePath)
	ipservService := ipserv.NewService(ipservClient, iservCache)

	urlParser := vpnurl.NewParser()

	configStore := &singboxConfigStore{}

	u := updater.NewUpdater(happClient, ipservService, urlParser, configStore, slog.Default())

	slog.Info("fetching links")

	_, err = u.Run(ctx, conf)
	if err != nil {
		slog.Error("failed to update singbox config", "error", err.Error())
		os.Exit(1)
		return
	}
}

// singboxConfigStore реализует updater.ConfigStore через функции пакета singbox.
type singboxConfigStore struct{}

func (s *singboxConfigStore) LoadConfig(path string) (*singbox.Config, error) {
	return singbox.LoadConfig(path)
}

func (s *singboxConfigStore) SaveConfig(path string, cfg *singbox.Config) error {
	return singbox.SaveConfig(path, cfg)
}

func (s *singboxConfigStore) CreateBackup(configPath string) (string, error) {
	return singbox.CreateBackup(configPath)
}
