package ipserv

import (
	"context"
	"fmt"
	"time"
)

// IPLookup — интерфейс для определения страны по IP.
type IPLookup interface {
	CountryByIP(ctx context.Context, ip string) (*Location, error)
}

// cacheEntry представляет запись в кеше.
type cacheEntry struct {
	// Country — название страны.
	Country string `json:"country"`
	// Timestamp — время создания записи.
	Timestamp time.Time `json:"timestamp"`
}

// Service предоставляет информацию о стране с кешированием.
type Service struct {
	// client — клиент для запросов к API ip-api.com.
	client IPLookup
	// cache — хранилище кеша.
	cache Cache
}

// NewService создаёт новый сервис с кешированием.
func NewService(client IPLookup, cache Cache) *Service {
	return &Service{
		client: client,
		cache:  cache,
	}
}

// CountryName возвращает название страны для заданного IP (или текущего соединения).
// Результат кешируется для уменьшения количества запросов к API.
func (s *Service) CountryName(ctx context.Context, ip string) (string, error) {
	// Проверяем кеш.
	if entry, ok := s.readCache(ip); ok {
		return entry.Country, nil
	}

	// Делаем запрос к API.
	loc, err := s.client.CountryByIP(ctx, ip)
	if err != nil {
		return "", fmt.Errorf("lookup country: %w", err)
	}

	// Сохраняем в кеш.
	if err := s.writeCache(ip, loc.Country); err != nil {
		// Ошибка записи кеша не критична — просто возвращаем результат.
		return loc.Country, nil
	}

	return loc.Country, nil
}

// readCache читает кеш и ищет запись для указанного IP.
func (s *Service) readCache(ip string) (*cacheEntry, bool) {
	cache, err := s.cache.Read()
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
func (s *Service) writeCache(ip string, country string) error {
	cache, err := s.cache.Read()
	if err != nil {
		// Если чтение не удалось — создаём новый кеш.
		cache = make(map[string]cacheEntry)
	}

	// Добавляем/обновляем запись.
	cache[ip] = cacheEntry{
		Country:   country,
		Timestamp: time.Now(),
	}

	return s.cache.Write(cache)
}
