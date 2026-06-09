package ipserv

import (
	"context"
	"errors"
	"fmt"

	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
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
	log := logger.FromContext(ctx)

	if len(f.providers) == 0 {
		return nil, errors.New("no providers configured")
	}

	var errs []error
	for _, p := range f.providers {
		providerType := fmt.Sprintf("%T", p)
		log.Debug("trying geo provider", "provider", providerType, "ip", ip)

		loc, err := p.CountryByIP(ctx, ip)
		if err == nil {
			log.Debug("geo provider succeeded", "provider", providerType, "country", loc.Country)
			return loc, nil
		}

		log.Warn("geo provider failed", "provider", providerType, "error", err.Error(), "ip", ip)
		errs = append(errs, err)
	}

	log.Warn("all geo providers failed", "ip", ip, "attempts", len(f.providers))
	return nil, fmt.Errorf("all providers failed: %w", errors.Join(errs...))
}
