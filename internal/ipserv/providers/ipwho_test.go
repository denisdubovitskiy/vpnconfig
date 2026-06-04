package providers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPWhoIsClient_SuccessWithIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"country":"United States","success":true}`)

	c := NewIPWhoIsClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "United States", loc.Country)
	assert.Equal(t, "success", loc.Status)
}

func TestIPWhoIsClient_SuccessWithoutIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"country":"France","success":true}`)

	c := NewIPWhoIsClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "France", loc.Country)
}

func TestIPWhoIsClient_NonOKStatus(t *testing.T) {
	t.Parallel()

	srv := newStatusServer(t, http.StatusInternalServerError)

	c := NewIPWhoIsClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 500")
}

func TestIPWhoIsClient_APIError(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"success":false,"message":"invalid ip address"}`)

	c := NewIPWhoIsClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ip address")
}

func TestIPWhoIsClient_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `garbage`)

	c := NewIPWhoIsClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}
