package ipserv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cache определяет интерфейс для чтения и записи кеша.
type Cache interface {
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
	if err := os.MkdirAll(dir, 0755); err != nil {
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

	if err := os.WriteFile(c.path, encoded, 0644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}

	return nil
}
