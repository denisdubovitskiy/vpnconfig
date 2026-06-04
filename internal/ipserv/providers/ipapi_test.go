package providers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPAPIClient_SuccessWithIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"country":"Netherlands","error":false}`)

	c := NewIPAPIClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "Netherlands", loc.Country)
	assert.Equal(t, "success", loc.Status)
}

func TestIPAPIClient_SuccessWithoutIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"country":"Germany","error":false}`)

	c := NewIPAPIClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "Germany", loc.Country)
}

func TestIPAPIClient_NonOKStatus(t *testing.T) {
	t.Parallel()

	srv := newStatusServer(t, http.StatusForbidden)

	c := NewIPAPIClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 403")
}

func TestIPAPIClient_RateLimited(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"error":true,"reason":"RateLimited"}`)

	c := NewIPAPIClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "RateLimited")
}

func TestIPAPIClient_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `<html>cloudflare</html>`)

	c := NewIPAPIClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}
