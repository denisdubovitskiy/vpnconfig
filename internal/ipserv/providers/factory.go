package providers

import (
	"fmt"
	"net/http"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// Имена встроенных провайдеров. Используются в config.yaml
// для указания порядка fallback и выбора конкретных провайдеров.
const (
	NameIPAPICo   = "ipapi_co"
	NameIPAPICom  = "ip_api_com"
	NameIPWhoIs   = "ipwho_is"
	NameAPI2IPMe  = "api_2ip_me"
	NameAPIIPSb   = "api_ip_sb"
	NameFreeGeoIP = "freegeoip_app"
)

// DefaultNames возвращает имена всех встроенных провайдеров
// в порядке fallback по умолчанию. Первый элемент — самый предпочтительный.
//
// Из исследования /Users/dvdubovitskiy/personal/vpn/sandbox/ip.txt
// исключён api.hostip.info, потому что он возвращает только ISO-код страны
// (две буквы), а не полное название — не совпадает с форматом config.yaml.
func DefaultNames() []string {
	return []string{
		NameIPAPICo,
		NameIPAPICom,
		NameIPWhoIs,
		NameAPI2IPMe,
		NameAPIIPSb,
		NameFreeGeoIP,
	}
}

// NewByName создаёт провайдера по его имени.
//
// Возвращает ошибку с списком доступных имён, если имя неизвестно.
func NewByName(name string, httpClient *http.Client) (ipserv.IPLookup, error) {
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	switch name {
	case NameIPAPICo:
		return NewIPAPIClient(httpClient), nil
	case NameIPAPICom:
		return NewIPAPICOMClient(httpClient), nil
	case NameIPWhoIs:
		return NewIPWhoIsClient(httpClient), nil
	case NameAPI2IPMe:
		return NewTwoIPClient(httpClient), nil
	case NameAPIIPSb:
		return NewIPSBClient(httpClient), nil
	case NameFreeGeoIP:
		return NewFreeGeoIPClient(httpClient), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (available: %v)", name, DefaultNames())
	}
}
