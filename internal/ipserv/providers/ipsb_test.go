package providers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPSBClient_SuccessWithIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"country":"United States","ip":"8.8.8.8"}`)

	c := NewIPSBClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "United States", loc.Country)
	assert.Equal(t, "success", loc.Status)
}

func TestIPSBClient_SuccessWithoutIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"country":"Japan"}`)

	c := NewIPSBClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "Japan", loc.Country)
}

func TestIPSBClient_NonOKStatus(t *testing.T) {
	t.Parallel()

	srv := newStatusServer(t, http.StatusServiceUnavailable)

	c := NewIPSBClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 503")
}

func TestIPSBClient_EmptyCountry(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"ip":"8.8.8.8"}`)

	c := NewIPSBClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "api error")
}

func TestIPSBClient_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `garbage`)

	c := NewIPSBClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}
