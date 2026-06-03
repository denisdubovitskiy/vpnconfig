package http

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	// Проверяем, что клиент создаётся с дефолтным таймаутом.
	t.Run("default timeout", func(t *testing.T) {
		t.Parallel()

		// act
		client := NewClient()

		// assert
		assert.Equal(t, defaultTimeout, client.Timeout)
	})

	// Проверяем переопределение таймаута.
	t.Run("custom timeout", func(t *testing.T) {
		t.Parallel()

		// arrange
		const customTimeout = 5 * time.Second

		// act
		client := NewClient(WithTimeout(customTimeout))

		// assert
		assert.Equal(t, customTimeout, client.Timeout)
	})
}

func TestUserAgentTransport_RoundTrip(t *testing.T) {
	t.Parallel()

	const testUA = "CustomUA/1.0"

	// Проверяем установку кастомного User-Agent.
	t.Run("custom user-agent", func(t *testing.T) {
		t.Parallel()

		// arrange
		mrt := NewMockRoundTripper(t)
		client := NewClient(
			WithUserAgent(testUA),
			WithTransport(mrt),
		)

		mrt.EXPECT().
			RoundTrip(mock.Anything).
			Run(func(req *http.Request) {
				assert.Equal(t, testUA, req.Header.Get("User-Agent"))
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				},
				nil,
			)

		// act
		_, err := client.Get("http://example.com")

		// assert
		require.NoError(t, err)
	})

	// Проверяем ротацию User-Agent через генератор.
	t.Run("generator rotation", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockGen := NewMockUserAgentGenerator(t)
		mockGen.EXPECT().
			RandomUserAgent().
			Return(testUA, true)

		mrt := NewMockRoundTripper(t)
		client := NewClient(
			WithUserAgentGenerator(mockGen),
			WithTransport(mrt),
		)

		mrt.EXPECT().
			RoundTrip(mock.Anything).
			Run(func(req *http.Request) {
				assert.Equal(t, testUA, req.Header.Get("User-Agent"))
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				},
				nil,
			)

		// act
		_, err := client.Get("http://example.com")

		// assert
		require.NoError(t, err)
	})

	// Проверяем, что генератор не устанавливает заголовок при false.
	t.Run("generator returns false skips header", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockGen := NewMockUserAgentGenerator(t)
		mockGen.EXPECT().
			RandomUserAgent().
			Return("", false)

		mrt := NewMockRoundTripper(t)
		client := NewClient(
			WithUserAgentGenerator(mockGen),
			WithTransport(mrt),
		)

		mrt.EXPECT().
			RoundTrip(mock.Anything).
			Run(func(req *http.Request) {
				assert.Empty(t, req.Header.Get("User-Agent"))
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				},
				nil,
			)

		// act
		_, err := client.Get("http://example.com")

		// assert
		require.NoError(t, err)
	})

	// Проверяем приоритет: кастомный UA должен переопределять генератор.
	t.Run("custom ua overrides generator", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockGen := NewMockUserAgentGenerator(t)
		mrt := NewMockRoundTripper(t)
		client := NewClient(
			WithUserAgent(testUA),
			WithUserAgentGenerator(mockGen),
			WithTransport(mrt),
		)

		mrt.EXPECT().
			RoundTrip(mock.Anything).
			Run(func(req *http.Request) {
				assert.Equal(t, testUA, req.Header.Get("User-Agent"))
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				},
				nil,
			)

		// act
		_, err := client.Get("http://example.com")

		// assert
		require.NoError(t, err)
		mockGen.AssertNotCalled(t, "RandomUserAgent")
	})

	// Проверяем, что запрос клонируется и не мутирует оригинал.
	t.Run("request is cloned", func(t *testing.T) {
		t.Parallel()

		// arrange
		const originalUA = "OriginalUA/1.0"
		mrt := NewMockRoundTripper(t)
		client := NewClient(
			WithUserAgent(testUA),
			WithTransport(mrt),
		)

		mrt.EXPECT().
			RoundTrip(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				},
				nil,
			)

		req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", originalUA)

		// act
		_, err = client.Do(req)

		// assert
		require.NoError(t, err)
		assert.Equal(t, originalUA, req.Header.Get("User-Agent"))
	})
}
