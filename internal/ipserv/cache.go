package ipserv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
)

// Storage определяет интерфейс для чтения и записи кеша.
type Storage interface {
	// Read читает весь кеш и возвращает мапу записей.
	Read() (map[string]cacheEntry, error)
	// Write записывает весь кеш в хранилище.
	Write(data map[string]cacheEntry) error
}

// Option — функциональная опция для настройки FileCache.
type Option func(*FileCache)

// WithTTL задаёт время жизни записей в кеше.
func WithTTL(d time.Duration) Option {
	return func(c *FileCache) {
		c.ttl = d
	}
}

// FileCache — файловая реализация кеша на диске.
type FileCache struct {
	// path — путь к файлу кеша.
	path string
	// ttl — время жизни записей. 0 означает отсутствие TTL.
	ttl time.Duration
	// nowFn возвращает текущее время. Переопределяется в тестах.
	nowFn func() time.Time
}

// NewFileCache создаёт новый файловый кеш.
func NewFileCache(path string, opts ...Option) *FileCache {
	c := &FileCache{
		path:  path,
		nowFn: time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Read читает кеш из файла.
// Если задан TTL — возвращает только актуальные записи.
func (c *FileCache) Read() (map[string]cacheEntry, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]cacheEntry), nil
		}
		return nil, fmt.Errorf("read cache file: %w", err)
	}

	var cache map[string]cacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("unmarshal cache: %w", err)
	}

	if c.ttl > 0 {
		now := c.nowFn()
		for ip, entry := range cache {
			if now.Sub(entry.Timestamp) > c.ttl {
				delete(cache, ip)
			}
		}
	}

	return cache, nil
}

// Write записывает кеш в файл.
// Если задан TTL — очищает устаревшие записи перед записью.
func (c *FileCache) Write(data map[string]cacheEntry) error {
	// Создаём директорию, если её нет.
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	if c.ttl > 0 {
		now := c.nowFn()
		for ip, entry := range data {
			if now.Sub(entry.Timestamp) > c.ttl {
				delete(data, ip)
			}
		}
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	if err := os.WriteFile(c.path, encoded, 0o644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}

	return nil
}

// CachedIPLookup оборачивает IPLookup с кешированием результатов.
// Реализует интерфейс IPLookup, поэтому может использоваться как декоратор.
type CachedIPLookup struct {
	provider IPLookup
	storage  Storage
	nowFn    func() time.Time
}

// NewCachedIPLookup создаёт кеширующий декоратор для IPLookup.
func NewCachedIPLookup(provider IPLookup, storage Storage) *CachedIPLookup {
	return &CachedIPLookup{
		provider: provider,
		storage:  storage,
		nowFn:    time.Now,
	}
}

// CountryName возвращает название страны для заданного IP.
// Обёртка над CountryByIP для совместимости с updater.GeoIPService.
func (c *CachedIPLookup) CountryName(ctx context.Context, ip string) (string, error) {
	loc, err := c.CountryByIP(ctx, ip)
	if err != nil {
		return "", err
	}
	return loc.Country, nil
}

// CountryByIP возвращает название страны для заданного IP.
// Сначала проверяет кеш, если нет — делегирует provider, затем сохраняет результат.
func (c *CachedIPLookup) CountryByIP(ctx context.Context, ip string) (*Location, error) {
	log := logger.FromContext(ctx)

	// Проверяем кеш.
	if entry, ok := c.readCache(ip); ok {
		log.Debug("geo cache hit", "ip", ip, "country", entry.Country)
		return &Location{
			Status:  "success",
			Country: entry.Country,
			Query:   ip,
		}, nil
	}

	log.Debug("geo cache miss", "ip", ip)

	// Делаем запрос к провайдеру.
	loc, err := c.provider.CountryByIP(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("lookup country: %w", err)
	}

	// Сохраняем в кеш.
	if err := c.writeCache(ip, loc.Country); err != nil {
		log.Warn("failed to write geo cache", "ip", ip, "error", err.Error())
		// Ошибка записи кеша не критична — просто возвращаем результат.
		return loc, nil
	}

	return loc, nil
}

// readCache читает кеш и ищет запись для указанного IP.
func (c *CachedIPLookup) readCache(ip string) (*cacheEntry, bool) {
	cache, err := c.storage.Read()
	if err != nil {
		return nil, false
	}

	entry, ok := cache[ip]
	if !ok {
		return nil, false
	}

	return &entry, true
}

// writeCache читает существующий кеш, добавляет/обновляет запись и сохраняет.
func (c *CachedIPLookup) writeCache(ip, country string) error {
	cache, err := c.storage.Read()
	if err != nil {
		// Если чтение не удалось — создаём новый кеш.
		cache = make(map[string]cacheEntry)
	}

	// Добавляем/обновляем запись.
	cache[ip] = cacheEntry{
		Country:   country,
		Timestamp: c.nowFn(),
	}

	return c.storage.Write(cache)
}
