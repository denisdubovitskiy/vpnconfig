package vpnurl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestVlessParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &VlessParser{}

	// Проверяем парсинг vless URL с reality.
	t.Run("vless with reality", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vless://uuid@192.0.2.1:8444?security=reality&sni=test.example.com&fp=chrome&pbk=test-public-key&sid=test-short-id&flow=xtls-rprx-vision"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "vless", outbound.Type())

		vless, ok := outbound.(*VLESSOutbound)
		require.True(t, ok)
		assert.Equal(t, "192.0.2.1", vless.Server)
		assert.Equal(t, 8444, vless.ServerPort)
		assert.Equal(t, "uuid", vless.UUID)
		assert.Equal(t, "xtls-rprx-vision", vless.Flow)
		require.NotNil(t, vless.TLS)
		assert.True(t, vless.TLS.Enabled)
		assert.Equal(t, "test.example.com", vless.TLS.ServerName)
		require.NotNil(t, vless.TLS.UTLS)
		assert.True(t, vless.TLS.UTLS.Enabled)
		assert.Equal(t, "chrome", vless.TLS.UTLS.Fingerprint)
		require.NotNil(t, vless.TLS.Reality)
		assert.True(t, vless.TLS.Reality.Enabled)
		assert.Equal(t, "test-public-key", vless.TLS.Reality.PublicKey)
		assert.Equal(t, "test-short-id", vless.TLS.Reality.ShortID)
	})

	// Проверяем парсинг vless URL с grpc транспортом.
	t.Run("vless with grpc", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vless://uuid@192.0.2.1:2053?security=reality&sni=test.example.com&fp=chrome&pbk=test-public-key&sid=test-short-id&type=grpc&serviceName=xyz"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		vless, ok := outbound.(*VLESSOutbound)
		require.True(t, ok)
		require.NotNil(t, vless.Transport)
		assert.Equal(t, "grpc", vless.Transport.Type)
		assert.Equal(t, "xyz", vless.Transport.ServiceName)
	})

	// Проверяем парсинг vless без flow.
	t.Run("vless without flow", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vless://uuid@192.0.2.1:8444?security=reality&sni=test.example.com"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		vless, ok := outbound.(*VLESSOutbound)
		require.True(t, ok)
		assert.Empty(t, vless.Flow)
	})

	// Проверяем парсинг network параметра.
	t.Run("vless with network", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vless://uuid@192.0.2.1:8444?security=tls&network=udp"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		vless, ok := outbound.(*VLESSOutbound)
		require.True(t, ok)
		assert.Equal(t, "udp", vless.Network)
	})

	// Проверяем что type=tcp не создаёт transport.
	t.Run("vless tcp without transport", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vless://uuid@192.0.2.1:8444?security=tls&type=tcp"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		vless, ok := outbound.(*VLESSOutbound)
		require.True(t, ok)
		assert.Nil(t, vless.Transport)
	})

	// Проверяем парсинг host для transport.
	t.Run("vless with host", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "vless://uuid@192.0.2.1:8444?security=tls&type=ws&host=cdn.example.com"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		vless, ok := outbound.(*VLESSOutbound)
		require.True(t, ok)
		require.NotNil(t, vless.Transport)
		assert.Equal(t, "cdn.example.com", vless.Transport.Host)
	})
}

func TestTrojanParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &TrojanParser{}

	// Проверяем парсинг trojan URL с ws транспортом.
	t.Run("trojan with ws", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "trojan://test-password@192.0.2.1:2058?security=tls&sni=test.example.com&type=ws&path=/"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "trojan", outbound.Type())

		trojan, ok := outbound.(*TrojanOutbound)
		require.True(t, ok)
		assert.Equal(t, "192.0.2.1", trojan.Server)
		assert.Equal(t, 2058, trojan.ServerPort)
		assert.Equal(t, "test-password", trojan.Password)
		require.NotNil(t, trojan.TLS)
		assert.True(t, trojan.TLS.Enabled)
		assert.Equal(t, "test.example.com", trojan.TLS.ServerName)
		require.NotNil(t, trojan.Transport)
		assert.Equal(t, "ws", trojan.Transport.Type)
		assert.Equal(t, "/", trojan.Transport.Path)
	})

	// Проверяем парсинг trojan без явного security (должен быть tls по умолчанию).
	t.Run("trojan default tls", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "trojan://password@example.com:443"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		trojan, ok := outbound.(*TrojanOutbound)
		require.True(t, ok)
		require.NotNil(t, trojan.TLS)
		assert.True(t, trojan.TLS.Enabled)
	})

	// Проверяем что type=tcp не создаёт transport для trojan.
	t.Run("trojan tcp without transport", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "trojan://password@example.com:443?type=tcp"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		trojan, ok := outbound.(*TrojanOutbound)
		require.True(t, ok)
		assert.Nil(t, trojan.Transport)
	})

	// Проверяем парсинг network параметра для trojan.
	t.Run("trojan with network", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "trojan://password@example.com:443?network=udp"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		trojan, ok := outbound.(*TrojanOutbound)
		require.True(t, ok)
		assert.Equal(t, "udp", trojan.Network)
	})

	// Проверяем парсинг host для transport.
	t.Run("trojan with host", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "trojan://password@example.com:443?type=ws&host=cdn.example.com"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)

		trojan, ok := outbound.(*TrojanOutbound)
		require.True(t, ok)
		require.NotNil(t, trojan.Transport)
		assert.Equal(t, "cdn.example.com", trojan.Transport.Host)
	})
}

func TestShadowsocksParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &ShadowsocksParser{}

	// Проверяем парсинг shadowsocks URL.
	t.Run("shadowsocks", func(t *testing.T) {
		t.Parallel()

		// arrange
		url := "ss://chacha20-ietf-poly1305:test-password@192.0.2.1:2060"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "shadowsocks", outbound.Type())

		ss, ok := outbound.(*ShadowsocksOutbound)
		require.True(t, ok)
		assert.Equal(t, "192.0.2.1", ss.Server)
		assert.Equal(t, 2060, ss.ServerPort)
		assert.Equal(t, "chacha20-ietf-poly1305", ss.Method)
		assert.Equal(t, "test-password", ss.Password)
	})

	// Проверяем парсинг shadowsocks URL с base64-encoded credentials.
	t.Run("shadowsocks base64", func(t *testing.T) {
		t.Parallel()

		// arrange
		// base64("chacha20-ietf-poly1305:test-password") = "Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTp0ZXN0LXBhc3N3b3Jk"
		url := "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTp0ZXN0LXBhc3N3b3Jk@192.0.2.1:2060"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "shadowsocks", outbound.Type())

		ss, ok := outbound.(*ShadowsocksOutbound)
		require.True(t, ok)
		assert.Equal(t, "192.0.2.1", ss.Server)
		assert.Equal(t, 2060, ss.ServerPort)
		assert.Equal(t, "chacha20-ietf-poly1305", ss.Method)
		assert.Equal(t, "test-password", ss.Password)
	})
}

func TestParser_Parse(t *testing.T) {
	t.Parallel()

	// Проверяем что Parser делегирует парсинг нужному SchemeParser.
	t.Run("delegates to scheme parser", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockParser := NewMockSchemeParser(t)
		expectedOutbound := &VLESSOutbound{OutboundType: "vless"}
		mockParser.EXPECT().
			Parse("vless://test").
			Return(expectedOutbound, nil)

		parser := NewParserWithParsers(map[string]SchemeParser{
			"vless": mockParser,
		})

		// act
		outbound, err := parser.Parse("vless://test")

		// assert
		require.NoError(t, err)
		assert.Equal(t, expectedOutbound, outbound)
	})

	// Проверяем ошибку при отсутствии схемы.
	t.Run("no scheme", func(t *testing.T) {
		t.Parallel()

		// arrange
		parser := NewParser()

		// act
		_, err := parser.Parse("not-a-url")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no scheme")
	})

	// Проверяем ошибку при неподдерживаемой схеме.
	t.Run("unsupported scheme", func(t *testing.T) {
		t.Parallel()

		// arrange
		parser := NewParser()

		// act
		_, err := parser.Parse("unknown://example.com")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported scheme")
	})

	// Проверяем что дефолтный парсер поддерживает все схемы.
	t.Run("default parser supports vless", func(t *testing.T) {
		t.Parallel()

		// arrange
		parser := NewParser()
		url := "vless://uuid@192.0.2.1:8444"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "vless", outbound.Type())
	})

	// Проверяем что дефолтный парсер поддерживает trojan.
	t.Run("default parser supports trojan", func(t *testing.T) {
		t.Parallel()

		// arrange
		parser := NewParser()
		url := "trojan://password@example.com:443"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "trojan", outbound.Type())
	})

	// Проверяем что дефолтный парсер поддерживает shadowsocks.
	t.Run("default parser supports shadowsocks", func(t *testing.T) {
		t.Parallel()

		// arrange
		parser := NewParser()
		url := "ss://method:password@example.com:8388"

		// act
		outbound, err := parser.Parse(url)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "shadowsocks", outbound.Type())
	})
}

func TestToOutbound(t *testing.T) {
	t.Parallel()

	// Проверяем сериализацию VLESS outbound в JSON.
	t.Run("vless json serialization", func(t *testing.T) {
		t.Parallel()

		// arrange
		outbound := &VLESSOutbound{
			OutboundType: "vless",
			OutboundTag:  "test-out",
			Server:       "192.0.2.1",
			ServerPort:   8444,
			UUID:         "uuid",
			Flow:         "xtls-rprx-vision",
			TLS: &TLSConfig{
				Enabled:    true,
				ServerName: "test.example.com",
				UTLS: &UTLSConfig{
					Enabled:     true,
					Fingerprint: "chrome",
				},
				Reality: &RealityConfig{
					Enabled:   true,
					PublicKey: "n-8UD2RECrSEi9_TkLrP3Cjf_sRdWDtQxftthTdupBk",
					ShortID:   "ff48391ffceb6947",
				},
			},
		}

		// act
		data, err := json.Marshal(outbound.ToOutbound())

		// assert
		require.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		assert.Equal(t, "vless", result["type"])
		assert.Equal(t, "test-out", result["tag"])
		assert.Equal(t, "192.0.2.1", result["server"])
		assert.Equal(t, float64(8444), result["server_port"])
		assert.Equal(t, "uuid", result["uuid"])
		assert.Equal(t, "xtls-rprx-vision", result["flow"])
	})
}

func TestParser_Parse_MultipleURLs(t *testing.T) {
	t.Parallel()

	// Проверяем что Parser корректно обрабатывает несколько URL разных схем.
	t.Run("parses multiple urls", func(t *testing.T) {
		t.Parallel()

		// arrange
		parser := NewParser()
		urls := []string{
			"vless://uuid@192.0.2.1:8444",
			"trojan://password@example.com:443",
			"ss://method:password@example.com:8388",
		}

		// act & assert
		for _, url := range urls {
			outbound, err := parser.Parse(url)
			require.NoError(t, err)
			assert.NotNil(t, outbound)
		}
	})
}

func TestParser_Parse_ErrorPropagation(t *testing.T) {
	t.Parallel()

	// Проверяем что ошибки от SchemeParser пробрасываются.
	t.Run("propagates parser error", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockParser := NewMockSchemeParser(t)
		mockParser.EXPECT().
			Parse(mock.Anything).
			Return(nil, assert.AnError)

		parser := NewParserWithParsers(map[string]SchemeParser{
			"vless": mockParser,
		})

		// act
		_, err := parser.Parse("vless://test")

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
