package ipserv

import (
	"context"
	"time"
)

// IPLookup — интерфейс для определения страны по IP.
type IPLookup interface {
	CountryByIP(ctx context.Context, ip string) (*Location, error)
}

// Location представляет результат определения страны по IP.
// Является общим форматом ответа для всех провайдеров в подпакете providers.
type Location struct {
	// Status — статус ответа ("success" при удаче).
	Status string `json:"status"`
	// Country — название страны.
	Country string `json:"country"`
	// Query — запрашиваемый IP-адрес.
	Query string `json:"query"`
	// Message — сообщение об ошибке (при неуспешном ответе).
	Message string `json:"message"`
}

// cacheEntry представляет запись в кеше.
type cacheEntry struct {
	// Country — название страны.
	Country string `json:"country"`
	// Timestamp — время создания записи.
	Timestamp time.Time `json:"timestamp"`
}
