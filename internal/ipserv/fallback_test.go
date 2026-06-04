package ipserv_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/ipserv"
)

// stubLookup — тестовая реализация IPLookup с заранее заданными результатами.
type stubLookup struct {
	location *ipserv.Location
	err      error
	calls    int
}

func (s *stubLookup) CountryByIP(_ context.Context, _ string) (*ipserv.Location, error) {
	s.calls++
	return s.location, s.err
}

func TestFallback_FirstProviderSucceeds(t *testing.T) {
	t.Parallel()

	first := &stubLookup{location: &ipserv.Location{Country: "Sweden"}}
	second := &stubLookup{location: &ipserv.Location{Country: "Russia"}}
	third := &stubLookup{location: &ipserv.Location{Country: "USA"}}

	fb := ipserv.NewFallback([]ipserv.IPLookup{first, second, third})

	got, err := fb.CountryByIP(t.Context(), "1.2.3.4")

	require.NoError(t, err)
	assert.Equal(t, "Sweden", got.Country)
	assert.Equal(t, 1, first.calls)
	assert.Equal(t, 0, second.calls)
	assert.Equal(t, 0, third.calls)
}

func TestFallback_FallsBackOnError(t *testing.T) {
	t.Parallel()

	first := &stubLookup{err: errors.New("timeout")}
	second := &stubLookup{err: errors.New("rate limit")}
	third := &stubLookup{location: &ipserv.Location{Country: "Netherlands"}}

	fb := ipserv.NewFallback([]ipserv.IPLookup{first, second, third})

	got, err := fb.CountryByIP(t.Context(), "1.2.3.4")

	require.NoError(t, err)
	assert.Equal(t, "Netherlands", got.Country)
	assert.Equal(t, 1, first.calls)
	assert.Equal(t, 1, second.calls)
	assert.Equal(t, 1, third.calls)
}

func TestFallback_AllProvidersFail(t *testing.T) {
	t.Parallel()

	first := &stubLookup{err: errors.New("err1")}
	second := &stubLookup{err: errors.New("err2")}
	third := &stubLookup{err: errors.New("err3")}

	fb := ipserv.NewFallback([]ipserv.IPLookup{first, second, third})

	_, err := fb.CountryByIP(t.Context(), "1.2.3.4")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed")
	assert.Contains(t, err.Error(), "err1")
	assert.Contains(t, err.Error(), "err2")
	assert.Contains(t, err.Error(), "err3")
}

func TestFallback_EmptyProvidersList(t *testing.T) {
	t.Parallel()

	fb := ipserv.NewFallback(nil)

	_, err := fb.CountryByIP(t.Context(), "1.2.3.4")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no providers configured")
}

func TestFallback_SingleProvider(t *testing.T) {
	t.Parallel()

	only := &stubLookup{location: &ipserv.Location{Country: "Germany"}}

	fb := ipserv.NewFallback([]ipserv.IPLookup{only})

	got, err := fb.CountryByIP(t.Context(), "1.2.3.4")

	require.NoError(t, err)
	assert.Equal(t, "Germany", got.Country)
	assert.Equal(t, 1, only.calls)
}
