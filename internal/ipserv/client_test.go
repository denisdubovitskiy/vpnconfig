package ipserv

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newTestClient создаёт клиента с тестовым HTTP-моком.
func newTestClient(t *testing.T) (*Client, *MockDoer) {
	t.Helper()

	mockDoer := NewMockDoer(t)
	client := NewClient(mockDoer)

	return client, mockDoer
}

// responseBody создаёт io.ReadCloser из строки для HTTP-ответа.
func responseBody(t *testing.T, body string) io.ReadCloser {
	t.Helper()

	return io.NopCloser(strings.NewReader(body))
}

func TestClient_CountryByIP(t *testing.T) {
	t.Parallel()

	const testIP = "192.0.2.1"

	// Проверяем успешный запрос с указанным IP.
	t.Run("success with ip", func(t *testing.T) {
		t.Parallel()

		// arrange
		client, mockDoer := newTestClient(t)
		mockDoer.EXPECT().
			Do(mock.Anything).
			Run(func(req *http.Request) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, "http://ip-api.com/json/192.0.2.1", req.URL.String())
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       responseBody(t, `{"status":"success","country":"Russia","query":"192.0.2.1"}`),
				},
				nil,
			)

		// act
		loc, err := client.CountryByIP(t.Context(), testIP)

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, "Russia", loc.Country)
		assert.Equal(t, "192.0.2.1", loc.Query)
	})

	// Проверяем успешный запрос без IP (автоопределение).
	t.Run("success without ip", func(t *testing.T) {
		t.Parallel()

		// arrange
		client, mockDoer := newTestClient(t)
		mockDoer.EXPECT().
			Do(mock.Anything).
			Run(func(req *http.Request) {
				require.Equal(t, "http://ip-api.com/json", req.URL.String())
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       responseBody(t, `{"status":"success","country":"USA","query":"198.51.100.1"}`),
				},
				nil,
			)

		// act
		loc, err := client.CountryByIP(t.Context(), "")

		// assert
		require.NoError(t, err)
		require.NotNil(t, loc)
		assert.Equal(t, "USA", loc.Country)
		assert.Equal(t, "198.51.100.1", loc.Query)
	})

	// Проверяем обработку невалидного HTTP-статуса.
	t.Run("non-ok status code", func(t *testing.T) {
		t.Parallel()

		// arrange
		client, mockDoer := newTestClient(t)
		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       responseBody(t, ""),
				},
				nil,
			)

		// act
		_, err := client.CountryByIP(t.Context(), testIP)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 429")
	})

	// Проверяем обработку ошибки сети.
	t.Run("network error", func(t *testing.T) {
		t.Parallel()

		// arrange
		client, mockDoer := newTestClient(t)
		wantErr := errors.New("connection refused")
		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(nil, wantErr)

		// act
		_, err := client.CountryByIP(t.Context(), testIP)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)
		assert.Contains(t, err.Error(), "execute request")
	})

	// Проверяем обработку невалидного JSON.
	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		// arrange
		client, mockDoer := newTestClient(t)
		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       responseBody(t, "not-json"),
				},
				nil,
			)

		// act
		_, err := client.CountryByIP(t.Context(), testIP)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode response")
	})

	// Проверяем обработку ошибки от API (status != success).
	t.Run("api error response", func(t *testing.T) {
		t.Parallel()

		// arrange
		client, mockDoer := newTestClient(t)
		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       responseBody(t, `{"status":"fail","message":"invalid query"}`),
				},
				nil,
			)

		// act
		_, err := client.CountryByIP(t.Context(), testIP)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "api error: invalid query")
	})
}
