package providers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPAPICOMClient_SuccessWithIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"success","country":"Sweden","query":"8.8.8.8"}`)

	c := NewIPAPICOMClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "Sweden", loc.Country)
	assert.Equal(t, "success", loc.Status)
	assert.Equal(t, "8.8.8.8", loc.Query)
}

func TestIPAPICOMClient_SuccessWithoutIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"success","country":"USA","query":"1.1.1.1"}`)

	c := NewIPAPICOMClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "USA", loc.Country)
}

func TestIPAPICOMClient_NonOKStatus(t *testing.T) {
	t.Parallel()

	srv := newStatusServer(t, http.StatusTooManyRequests)

	c := NewIPAPICOMClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 429")
}

func TestIPAPICOMClient_APIError(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"fail","message":"private range"}`)

	c := NewIPAPICOMClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "192.168.0.1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "private range")
}

func TestIPAPICOMClient_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `not-json`)

	c := NewIPAPICOMClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}
