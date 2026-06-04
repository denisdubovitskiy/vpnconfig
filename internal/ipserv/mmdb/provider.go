// Package mmdb предоставляет провайдер определения страны по IP
// на основе локальной MMDB-базы MaxMind GeoLite2-Country.
//
// Провайдер опционален и активируется через config.MMDBConfig.
// При первом запуске файл базы автоматически скачивается по DownloadURL
// с созданием всех промежуточных директорий.
package mmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"

	"github.com/oschwald/geoip2-golang/v2"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// GeoReader — минимальный интерфейс, который провайдер использует для
// обращения к MMDB-базе. Реализуется *geoip2.Reader. Выделен отдельно
// для подмены в тестах моком и для явного контракта на освобождение
// ресурсов через io.Closer.
type GeoReader interface {
	Country(netip.Addr) (*geoip2.Country, error)
	io.Closer
}

// HTTPDoer — минимальный контракт HTTP-клиента, который использует mmdb
// для скачивания базы. Реализуется *http.Client из stdlib, что позволяет
// передавать его без адаптеров, и легко мокается в тестах. Возвращённый
// *http.Response должен иметь не-nil Body, который mmdb закрывает после
// чтения; вызывающий код отвечает за закрытие транспорта (idle connections)
// по завершении работы, если это требуется.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Provider — провайдер определения страны по IP через локальную MMDB-базу.
// Реализует интерфейс ipserv.IPLookup. Логгер передаётся через ctx
// (см. internal/logger) — отдельной зависимости у структуры нет.
type Provider struct {
	// reader — источник данных о странах. Закрывается через Close.
	reader GeoReader
}

// New создаёт MMDB-провайдер. Если файл базы отсутствует на диске, он
// будет скачан по cfg.EffectiveDownloadURL() с созданием всех промежуточных
// директорий. Полученный провайдер должен быть закрыт через Close.
// Логгер должен быть передан через ctx (logger.IntoContext) — при его
// отсутствии используется slog.Default().
func New(
	ctx context.Context,
	cfg *config.MMDBConfig,
	client HTTPDoer,
) (*Provider, error) {
	if cfg == nil {
		return nil, ErrConfigNil
	}
	if !cfg.Enabled {
		return nil, ErrNotEnabled
	}
	if cfg.DatabasePath == "" {
		return nil, ErrDatabasePathEmpty
	}
	if client == nil {
		return nil, ErrClientNil
	}

	if err := ensureDatabase(ctx, cfg, client); err != nil {
		return nil, err
	}

	db, err := geoip2.Open(cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open mmdb database: %w", err)
	}

	return &Provider{reader: db}, nil
}

// Close закрывает открытую MMDB-базу и освобождает занятые ею ресурсы.
// Безопасен для вызова на nil-получателе.
func (p *Provider) Close() error {
	if p == nil || p.reader == nil {
		return nil
	}
	return p.reader.Close()
}

// CountryByIP возвращает название страны для заданного IP-адреса.
// Если в базе нет данных (например, для приватных адресов), возвращает
// ошибку, обёрнутую вокруг ErrNoData.
func (p *Provider) CountryByIP(_ context.Context, ip string) (*ipserv.Location, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return nil, errors.Join(ErrParseIP, err)
	}

	record, err := p.reader.Country(addr)
	if err != nil {
		return nil, fmt.Errorf("lookup country: %w", err)
	}

	if record == nil || !record.HasData() {
		return nil, fmt.Errorf("%w: %s", ErrNoData, ip)
	}

	country := record.Country.Names.English
	if country == "" {
		country = record.Country.ISOCode
	}

	return &ipserv.Location{
		Status:  "success",
		Country: country,
		Query:   ip,
	}, nil
}
