package ipserv

import (
	"context"
	"errors"
	"fmt"
)

// Fallback последовательно перебирает несколько IPLookup и возвращает
// первый успешный результат. Если все провайдеры упали — возвращает
// агрегированную ошибку через errors.Join.
type Fallback struct {
	providers []IPLookup
}

// NewFallback создаёт новый Fallback из списка провайдеров.
// Порядок провайдеров определяет приоритет: первый элемент —
// самый предпочтительный, последний — fallback последней надежды.
func NewFallback(providers []IPLookup) *Fallback {
	return &Fallback{providers: providers}
}

// CountryByIP перебирает провайдеры по порядку и возвращает первый
// успешный результат. Если все вернули ошибку — возвращает ошибку
// со списком всех причин.
func (f *Fallback) CountryByIP(ctx context.Context, ip string) (*Location, error) {
	if len(f.providers) == 0 {
		return nil, errors.New("no providers configured")
	}

	var errs []error
	for _, p := range f.providers {
		loc, err := p.CountryByIP(ctx, ip)
		if err == nil {
			return loc, nil
		}
		errs = append(errs, err)
	}

	return nil, fmt.Errorf("all providers failed: %w", errors.Join(errs...))
}
