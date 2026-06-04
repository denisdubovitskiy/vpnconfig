package providers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFreeGeoIPClient_SuccessWithIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"ip":"8.8.8.8","country_name":"United States","country_code":"US"}`)

	c := NewFreeGeoIPClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "United States", loc.Country)
	assert.Equal(t, "success", loc.Status)
}

func TestFreeGeoIPClient_SuccessWithoutIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"ip":"1.1.1.1","country_name":"Australia"}`)

	c := NewFreeGeoIPClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "Australia", loc.Country)
}

func TestFreeGeoIPClient_Forbidden(t *testing.T) {
	t.Parallel()

	srv := newStatusServer(t, http.StatusForbidden)

	c := NewFreeGeoIPClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 403")
}

func TestFreeGeoIPClient_EmptyCountryName(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"ip":"8.8.8.8","country_name":""}`)

	c := NewFreeGeoIPClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty country_name")
}

func TestFreeGeoIPClient_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `not-json`)

	c := NewFreeGeoIPClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}
