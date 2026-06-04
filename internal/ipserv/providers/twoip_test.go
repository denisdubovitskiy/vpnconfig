package providers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTwoIPClient_SuccessWithIP(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"country":"Russia","ip":"8.8.8.8"}`)

	c := NewTwoIPClient(srv.Client())
	c.baseURL = srv.URL

	loc, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.NoError(t, err)
	require.NotNil(t, loc)
	assert.Equal(t, "Russia", loc.Country)
	assert.Equal(t, "success", loc.Status)
}

func TestTwoIPClient_NonOKStatus(t *testing.T) {
	t.Parallel()

	srv := newStatusServer(t, http.StatusTooManyRequests)

	c := NewTwoIPClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 429")
}

func TestTwoIPClient_EmptyCountry(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"ip":"8.8.8.8","country":""}`)

	c := NewTwoIPClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty country")
}

func TestTwoIPClient_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `not-json`)

	c := NewTwoIPClient(srv.Client())
	c.baseURL = srv.URL

	_, err := c.CountryByIP(t.Context(), "8.8.8.8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}
