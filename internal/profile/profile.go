package profile

import "context"

// LinkFetcher получает список VPN-ссылок из подписки.
type LinkFetcher interface {
	FetchLinks(ctx context.Context, url string) ([]string, error)
}
