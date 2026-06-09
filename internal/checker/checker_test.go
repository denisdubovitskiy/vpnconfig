package checker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/singbox"
	"github.com/denisdubovitskiy/vpnconfig/internal/vpnurl"
)

func newTestParser() *vpnurl.Parser {
	return vpnurl.NewParser()
}

var _ Checker = (*linkChecker)(nil)

const testVLESSLink = "vless://test-uuid@185.189.46.17:443?security=reality&sni=example.com&pbk=test-public-key&sid=test-short-id&flow=xtls-rprx-vision&type=tcp#TestServer"
const testPort = 12345

func newTestChecker(cfg Config, runner SingBoxRunner, httpMock HttpDoer, portGen PortGenerator) Checker {
	return NewChecker(cfg, newTestParser(), runner,
		WithHTTPDoer(httpMock),
		WithPortGenerator(portGen),
	)
}

func newMockPortGen(t *testing.T) *MockPortGenerator {
	t.Helper()
	gen := NewMockPortGenerator(t)
	gen.
		EXPECT().
		RandomPort(mock.Anything).
		Return(testPort, nil)
	return gen
}

func TestLinkChecker_CheckLink(t *testing.T) {
	t.Parallel()

	t.Run("parser error", func(t *testing.T) {
		t.Parallel()

		// arrange
		runnerMock := NewMockSingBoxRunner(t)
		httpMock := NewMockHttpDoer(t)
		portGen := NewMockPortGenerator(t) // RandomPort не вызывается при ошибке парсинга

		cfg := Config{
			Timeout: 5 * time.Second, // 5s
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), "invalid-url")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse vpn url")
	})

	t.Run("runner start error", func(t *testing.T) {
		t.Parallel()

		// arrange
		startErr := errors.New("sing-box not found")
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, testPort).
			Return(startErr)

		httpMock := NewMockHttpDoer(t)
		portGen := newMockPortGen(t)

		cfg := Config{
			Timeout: 5 * time.Second,
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start sing-box")
		assert.ErrorIs(t, err, startErr)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, testPort).
			Return(nil)
		runnerMock.
			EXPECT().
			Stop(mock.Anything).
			Return(nil)

		httpMock := NewMockHttpDoer(t)
		httpMock.
			EXPECT().
			Do(mock.Anything).
			Run(func(req *http.Request) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, defaultTestURL, req.URL.String())
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
				},
				nil,
			)

		portGen := newMockPortGen(t)

		cfg := Config{
			Timeout: 5 * time.Second,
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.NoError(t, err)
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		// arrange
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, testPort).
			Return(nil)
		runnerMock.
			EXPECT().
			Stop(mock.Anything).
			Return(nil)

		httpMock := NewMockHttpDoer(t)
		httpMock.
			EXPECT().
			Do(mock.Anything).
			Return(nil, context.Canceled)

		portGen := newMockPortGen(t)

		cfg := Config{
			Timeout: 5 * time.Second,
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		// act
		err := chk.CheckLink(ctx, testVLESSLink)

		// assert
		require.Error(t, err)
	})

	t.Run("http error", func(t *testing.T) {
		t.Parallel()

		// arrange
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, testPort).
			Return(nil)
		runnerMock.
			EXPECT().
			Stop(mock.Anything).
			Return(nil)

		httpMock := NewMockHttpDoer(t)
		httpMock.
			EXPECT().
			Do(mock.Anything).
			Return(nil, errors.New("connection refused"))

		portGen := newMockPortGen(t)

		cfg := Config{
			Timeout: 5 * time.Second,
			URLs:    []string{"http://example.com"},
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execute request")
	})

	t.Run("unexpected status code", func(t *testing.T) {
		t.Parallel()

		// arrange
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, testPort).
			Return(nil)
		runnerMock.
			EXPECT().
			Stop(mock.Anything).
			Return(nil)

		httpMock := NewMockHttpDoer(t)
		httpMock.
			EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader("")),
				},
				nil,
			)

		portGen := newMockPortGen(t)

		cfg := Config{
			Timeout: 5 * time.Second,
			URLs:    []string{"http://example.com"},
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code")
	})

	t.Run("default timeout", func(t *testing.T) {
		t.Parallel()

		// arrange
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, testPort).
			Return(nil)
		runnerMock.
			EXPECT().
			Stop(mock.Anything).
			Return(nil)

		httpMock := NewMockHttpDoer(t)
		httpMock.
			EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
				},
				nil,
			)

		portGen := newMockPortGen(t)

		cfg := Config{}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.NoError(t, err)
	})

	t.Run("port generation error", func(t *testing.T) {
		t.Parallel()

		// arrange
		portErr := errors.New("no free ports")
		runnerMock := NewMockSingBoxRunner(t)
		portGen := NewMockPortGenerator(t)
		portGen.
			EXPECT().
			RandomPort(mock.Anything).
			Return(0, portErr)

		httpMock := NewMockHttpDoer(t)

		cfg := Config{
			Timeout: 5 * time.Second,
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get random port")
		assert.ErrorIs(t, err, portErr)
	})

	t.Run("uses configured test urls", func(t *testing.T) {
		t.Parallel()

		// arrange
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, testPort).
			Return(nil)
		runnerMock.
			EXPECT().
			Stop(mock.Anything).
			Return(nil)

		customURLs := []string{
			"https://cp.cloudflare.com",
			"https://www.gstatic.com/generate_204",
		}

		httpMock := NewMockHttpDoer(t)
		httpMock.
			EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
				},
				nil,
			).Times(len(customURLs))

		portGen := newMockPortGen(t)

		cfg := Config{
			Timeout: 5 * time.Second,
			URLs:    customURLs,
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.NoError(t, err)
	})
}

func TestLinkChecker_CheckLink_ProxyURL(t *testing.T) {
	t.Parallel()

	t.Run("uses socks5 proxy on dynamic port", func(t *testing.T) {
		t.Parallel()

		// arrange
		dynamicPort := 15432
		runnerMock := NewMockSingBoxRunner(t)
		runnerMock.
			EXPECT().
			Start(mock.Anything, mock.Anything, dynamicPort).
			Return(nil)
		runnerMock.
			EXPECT().
			Stop(mock.Anything).
			Return(nil)

		portGen := NewMockPortGenerator(t)
		portGen.
			EXPECT().
			RandomPort(mock.Anything).
			Return(dynamicPort, nil)

		httpMock := NewMockHttpDoer(t)
		httpMock.
			EXPECT().
			Do(mock.Anything).
			Run(func(req *http.Request) {
				// http.Client использует transport.Proxy, который в тестах
				// проверяется через отдельный тест buildConfig. Здесь достаточно
				// убедиться, что запрос корректен.
				require.Equal(t, defaultTestURL, req.URL.String())
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
				},
				nil,
			)

		cfg := Config{
			Timeout: 5 * time.Second,
		}
		chk := newTestChecker(cfg, runnerMock, httpMock, portGen)

		// act
		err := chk.CheckLink(t.Context(), testVLESSLink)

		// assert
		require.NoError(t, err)
	})
}

func TestBuildConfig(t *testing.T) {
	t.Parallel()

	// arrange
	outbound := singbox.Outbound{
		"type":        "vless",
		"tag":         "test-out",
		"server":      "185.189.46.17",
		"server_port": 443,
		"uuid":        "test-uuid",
	}

	// act
	data, err := buildConfig(outbound, testPort)

	// assert
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal(data, &cfg))

	log, ok := cfg["log"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "error", log["level"])

	inbounds, ok := cfg["inbounds"].([]any)
	require.True(t, ok)
	require.Len(t, inbounds, 1)

	socksIn, ok := inbounds[0].(map[string]any)
	require.True(t, ok, "socks inbound should be map[string]any")
	assert.Equal(t, "socks", socksIn["type"])
	assert.Equal(t, socksInboundTag, socksIn["tag"])
	assert.Equal(t, "127.0.0.1", socksIn["listen"])
	assert.Equal(t, float64(testPort), socksIn["listen_port"])

	outbounds, ok := cfg["outbounds"].([]any)
	require.True(t, ok)
	assert.Len(t, outbounds, 2)

	vlessOut, ok := outbounds[0].(map[string]any)
	require.True(t, ok, "vless outbound should be map[string]any")
	assert.Equal(t, "vless", vlessOut["type"])
	assert.Equal(t, "test-out", vlessOut["tag"])

	directOut, ok := outbounds[1].(map[string]any)
	require.True(t, ok, "direct outbound should be map[string]any")
	assert.Equal(t, "direct", directOut["type"])

	route, ok := cfg["route"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-out", route["final"])
}

func TestInjectableClient(t *testing.T) {
	t.Parallel()

	t.Run("returns mock as is", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockDoer := NewMockHttpDoer(t)
		chk := &linkChecker{doer: mockDoer}

		// act
		client := chk.injectableClient(nil)

		// assert
		assert.Equal(t, mockDoer, client)
	})

	t.Run("wraps http client with proxy", func(t *testing.T) {
		t.Parallel()

		// arrange
		chk := &linkChecker{doer: defaultHTTPClient()}
		proxyURL, err := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", testPort))
		require.NoError(t, err)

		// act
		client := chk.injectableClient(proxyURL)

		// assert
		httpClient, ok := client.(*http.Client)
		require.True(t, ok)
		require.NotNil(t, httpClient.Transport)
		transport, ok := httpClient.Transport.(*http.Transport)
		require.True(t, ok)
		assert.NotNil(t, transport.Proxy)
	})
}
