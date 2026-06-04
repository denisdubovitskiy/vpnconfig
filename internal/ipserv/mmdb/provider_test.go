package mmdb

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/oschwald/geoip2-golang/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
)

func TestProvider_CountryByIP(t *testing.T) {
	t.Parallel()

	t.Run("returns english country name", func(t *testing.T) {
		t.Parallel()

		// arrange
		addr := netip.MustParseAddr(testIPGoogle)
		reader := NewMockGeoReader(t)
		reader.EXPECT().
			Country(addr).
			Return(&geoip2.Country{
				Country: geoip2.CountryRecord{
					ISOCode: "US",
					Names:   geoip2.Names{English: "United States"},
				},
			}, nil)

		p := &Provider{reader: reader}

		// act
		loc, err := p.CountryByIP(t.Context(), testIPGoogle)

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, "United States", loc.Country)
		assert.Equal(t, testIPGoogle, loc.Query)
		assert.Equal(t, "success", loc.Status)
	})

	t.Run("falls back to iso code if english name is empty", func(t *testing.T) {
		t.Parallel()

		// arrange
		addr := netip.MustParseAddr(testIPCloudflare)
		reader := NewMockGeoReader(t)
		reader.EXPECT().
			Country(addr).
			Return(&geoip2.Country{
				Country: geoip2.CountryRecord{
					ISOCode: "AU",
					Names:   geoip2.Names{English: ""},
				},
			}, nil)

		p := &Provider{reader: reader}

		// act
		loc, err := p.CountryByIP(t.Context(), testIPCloudflare)

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, "AU", loc.Country)
	})

	t.Run("returns ErrNoData when record has no data", func(t *testing.T) {
		t.Parallel()

		// arrange
		addr := netip.MustParseAddr(testIPPrivate)
		reader := NewMockGeoReader(t)
		reader.EXPECT().
			Country(addr).
			Return(&geoip2.Country{}, nil)

		p := &Provider{reader: reader}

		// act
		_, err := p.CountryByIP(t.Context(), testIPPrivate)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoData)
	})

	t.Run("returns ErrNoData when record is nil", func(t *testing.T) {
		t.Parallel()

		// arrange
		addr := netip.MustParseAddr(testIPPrivateRFC)
		reader := NewMockGeoReader(t)
		reader.EXPECT().
			Country(addr).
			Return(nil, nil)

		p := &Provider{reader: reader}

		// act
		_, err := p.CountryByIP(t.Context(), testIPPrivateRFC)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoData)
	})

	t.Run("wraps reader error", func(t *testing.T) {
		t.Parallel()

		// arrange
		addr := netip.MustParseAddr(testIPGoogle)
		innerErr := errors.New("reader failed")
		reader := NewMockGeoReader(t)
		reader.EXPECT().
			Country(addr).
			Return(nil, innerErr)

		p := &Provider{reader: reader}

		// act
		_, err := p.CountryByIP(t.Context(), testIPGoogle)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, innerErr)
	})

	t.Run("returns ErrParseIP on invalid ip", func(t *testing.T) {
		t.Parallel()

		// arrange
		reader := NewMockGeoReader(t)
		p := &Provider{reader: reader}

		// act
		_, err := p.CountryByIP(t.Context(), testIPInvalid)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrParseIP)
		reader.AssertNotCalled(t, "Country")
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("returns ErrConfigNil for nil config", func(t *testing.T) {
		t.Parallel()

		_, err := New(t.Context(), nil, nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrConfigNil)
	})

	t.Run("returns ErrNotEnabled when disabled", func(t *testing.T) {
		t.Parallel()

		cfg := &config.MMDBConfig{Enabled: false}

		_, err := New(t.Context(), cfg, nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotEnabled)
	})

	t.Run("returns ErrDatabasePathEmpty when path is empty", func(t *testing.T) {
		t.Parallel()

		cfg := &config.MMDBConfig{Enabled: true, DatabasePath: ""}

		_, err := New(t.Context(), cfg, nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDatabasePathEmpty)
	})

	t.Run("returns ErrClientNil for nil client", func(t *testing.T) {
		t.Parallel()

		cfg := newMMDBConfig("/tmp/any.mmdb", "")

		_, err := New(t.Context(), cfg, nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrClientNil)
	})
}

func TestProvider_Close(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for nil provider", func(t *testing.T) {
		t.Parallel()

		var p *Provider

		err := p.Close()

		require.NoError(t, err)
	})

	t.Run("returns nil when reader is nil", func(t *testing.T) {
		t.Parallel()

		p := &Provider{reader: nil}

		err := p.Close()

		require.NoError(t, err)
	})

	t.Run("delegates to reader Close", func(t *testing.T) {
		t.Parallel()

		// arrange
		reader := NewMockGeoReader(t)
		reader.EXPECT().Close().Return(nil)
		p := &Provider{reader: reader}

		// act
		err := p.Close()

		// assert
		require.NoError(t, err)
	})
}
