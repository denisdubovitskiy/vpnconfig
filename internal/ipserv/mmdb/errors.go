package mmdb

import "errors"

// Sentinel-ошибки, возвращаемые конструктором New. Экспортированы, чтобы
// вызывающий код мог различать ситуации через errors.Is без сравнения строк.
var (
	// ErrConfigNil — передан nil вместо *config.MMDBConfig.
	ErrConfigNil = errors.New("mmdb config is required")
	// ErrNotEnabled — провайдер не активирован в конфиге (Enabled == false).
	ErrNotEnabled = errors.New("mmdb provider is not enabled")
	// ErrDatabasePathEmpty — Enabled == true, но DatabasePath не задан.
	ErrDatabasePathEmpty = errors.New("mmdb database path is required")
	// ErrClientNil — передан nil вместо HTTPDoer.
	ErrClientNil = errors.New("http client is required")
	// ErrNoData — для IP нет данных в MMDB-базе (приватные/зарезервированные адреса).
	ErrNoData = errors.New("no mmdb data for ip")
	// ErrParseIP — переданную строку не удалось распарсить как IP-адрес.
	ErrParseIP = errors.New("parse ip")
)
