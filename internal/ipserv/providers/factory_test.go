package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv/providers"
)

// defaultNames содержит имена всех встроенных провайдеров
// в порядке, в котором они перебираются по умолчанию.
var defaultNames = []string{
	"ipapi_co",
	"ip_api_com",
	"ipwho_is",
	"api_2ip_me",
	"api_ip_sb",
	"freegeoip_app",
}

func TestNewByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		lookup func(t *testing.T, p ipserv.IPLookup)
	}{
		{
			name: "ipapi_co",
			//nolint:thelper // это callback, не helper-функция.
			lookup: func(t *testing.T, p ipserv.IPLookup) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"country":"Sweden"}`))
				}))
				t.Cleanup(srv.Close)

				// Подменяем базовый URL нельзя — конструктор принимает только Doer.
				// Проверяем только, что провайдер создан и удовлетворяет интерфейсу.
				require.NotNil(t, p)
			},
		},
		{
			name: "ip_api_com",
			//nolint:thelper // это callback, не helper-функция.
			lookup: func(t *testing.T, p ipserv.IPLookup) {
				require.NotNil(t, p)
			},
		},
		{
			name: "ipwho_is",
			//nolint:thelper // это callback, не helper-функция.
			lookup: func(t *testing.T, p ipserv.IPLookup) {
				require.NotNil(t, p)
			},
		},
		{
			name: "api_2ip_me",
			//nolint:thelper // это callback, не helper-функция.
			lookup: func(t *testing.T, p ipserv.IPLookup) {
				require.NotNil(t, p)
			},
		},
		{
			name: "api_ip_sb",
			//nolint:thelper // это callback, не helper-функция.
			lookup: func(t *testing.T, p ipserv.IPLookup) {
				require.NotNil(t, p)
			},
		},
		{
			name: "freegeoip_app",
			//nolint:thelper // это callback, не helper-функция.
			lookup: func(t *testing.T, p ipserv.IPLookup) {
				require.NotNil(t, p)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := providers.NewByName(tt.name, &http.Client{})
			require.NoError(t, err)
			require.NotNil(t, p)
			tt.lookup(t, p)
		})
	}
}

func TestNewByName_UnknownProvider(t *testing.T) {
	t.Parallel()

	p, err := providers.NewByName("nonexistent", &http.Client{})

	require.Error(t, err)
	assert.Nil(t, p)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestDefaultNames(t *testing.T) {
	t.Parallel()

	names := providers.DefaultNames()
	assert.Equal(t, defaultNames, names)
}

// TestDefaultNamesAllCreateable проверяет, что каждое имя из DefaultNames
// может быть использовано в NewByName без ошибки.
func TestDefaultNamesAllCreateable(t *testing.T) {
	t.Parallel()

	for _, name := range providers.DefaultNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p, err := providers.NewByName(name, &http.Client{})
			require.NoError(t, err)
			assert.NotNil(t, p)
		})
	}
}

// _ используется, чтобы гарантировать наличие импорта context в случае
// будущих расширений теста.
var _ = context.Background
