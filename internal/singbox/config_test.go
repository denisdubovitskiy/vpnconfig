package singbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	// Проверяем успешную загрузку конфига.
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "singbox.json")
		data := []byte(`{"outbounds":[{"type":"direct","tag":"direct-out"}]}`)
		err := os.WriteFile(path, data, 0o644)
		require.NoError(t, err)

		// act
		cfg, err := LoadConfig(path)

		// assert
		require.NoError(t, err)
		require.Len(t, cfg.Outbounds, 1)
		assert.Equal(t, "direct-out", cfg.Outbounds[0].Tag())
	})

	// Проверяем ошибку при отсутствии файла.
	t.Run("file not found", func(t *testing.T) {
		t.Parallel()

		// act
		_, err := LoadConfig("/nonexistent/singbox.json")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read singbox config")
	})

	// Проверяем ошибку при невалидном JSON.
	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "singbox.json")
		err := os.WriteFile(path, []byte("not json"), 0o644)
		require.NoError(t, err)

		// act
		_, err = LoadConfig(path)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse singbox config")
	})
}

func TestSaveConfig(t *testing.T) {
	t.Parallel()

	// Проверяем сохранение конфига.
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "singbox.json")
		cfg := &Config{
			Outbounds: []Outbound{
				{"type": "direct", "tag": "direct-out"},
			},
		}

		// act
		err := SaveConfig(path, cfg)

		// assert
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "direct-out")
	})
}

func TestConfig_RemoveSectionOutbounds(t *testing.T) {
	t.Parallel()

	// Проверяем удаление outbounds секции.
	t.Run("removes section outbounds", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{
			Outbounds: []Outbound{
				{"type": "direct", "tag": "direct-out"},
				{"type": "vless", "tag": "TEST-1-out"},
				{"type": "trojan", "tag": "TEST-2-out"},
				{"type": "urltest", "tag": "TEST-urltest-out"},
				{"type": "selector", "tag": "TEST-out"},
				{"type": "vless", "tag": "OTHER-1-out"},
			},
		}

		// act
		cfg.RemoveSectionOutbounds("TEST")

		// assert
		require.Len(t, cfg.Outbounds, 2)
		assert.Equal(t, "direct-out", cfg.Outbounds[0].Tag())
		assert.Equal(t, "OTHER-1-out", cfg.Outbounds[1].Tag())
	})

	// Проверяем, что другие секции не трогаются.
	t.Run("preserves other sections", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{
			Outbounds: []Outbound{
				{"type": "vless", "tag": "SEC1-1-out"},
				{"type": "vless", "tag": "SEC2-1-out"},
			},
		}

		// act
		cfg.RemoveSectionOutbounds("SEC1")

		// assert
		require.Len(t, cfg.Outbounds, 1)
		assert.Equal(t, "SEC2-1-out", cfg.Outbounds[0].Tag())
	})
}

func TestGenerateSectionOutbounds(t *testing.T) {
	t.Parallel()

	// Проверяем генерацию outbounds для секции.
	t.Run("generates section with proxies", func(t *testing.T) {
		t.Parallel()

		// arrange
		proxies := []Outbound{
			{"type": "vless", "server": "203.0.113.1"},
			{"type": "trojan", "server": "203.0.113.2"},
		}

		// act
		outbounds := GenerateSectionOutbounds("TEST", proxies, "https://test.com", "3m", 50)

		// assert
		require.Len(t, outbounds, 4) // 2 proxies + urltest + selector

		// Проверяем прокси.
		assert.Equal(t, "TEST-1-out", outbounds[0].Tag())
		assert.Equal(t, "vless", outbounds[0].Type())
		assert.Equal(t, "TEST-2-out", outbounds[1].Tag())
		assert.Equal(t, "trojan", outbounds[1].Type())

		// Проверяем urltest.
		assert.Equal(t, "urltest", outbounds[2].Type())
		assert.Equal(t, "TEST-urltest-out", outbounds[2].Tag())
		assert.Equal(t, []string{"TEST-1-out", "TEST-2-out"}, outbounds[2]["outbounds"])
		assert.Equal(t, "https://test.com", outbounds[2]["url"])
		assert.Equal(t, 50, outbounds[2]["tolerance"])

		// Проверяем selector.
		assert.Equal(t, "selector", outbounds[3].Type())
		assert.Equal(t, "TEST-out", outbounds[3].Tag())
		assert.Equal(t, []string{"TEST-1-out", "TEST-2-out", "TEST-urltest-out"}, outbounds[3]["outbounds"])
		assert.Equal(t, "TEST-urltest-out", outbounds[3]["default"])
	})
}

func TestOutbound_Tag(t *testing.T) {
	t.Parallel()

	// Проверяем получение тега.
	t.Run("returns tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := Outbound{"tag": "test-out"}

		// act & assert
		assert.Equal(t, "test-out", o.Tag())
	})

	// Проверяем пустой тег.
	t.Run("empty tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := Outbound{}

		// act & assert
		assert.Empty(t, o.Tag())
	})
}

func TestOutbound_SetTag(t *testing.T) {
	t.Parallel()

	// Проверяем установку тега.
	t.Run("sets tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := Outbound{}

		// act
		o.SetTag("new-tag")

		// assert
		assert.Equal(t, "new-tag", o.Tag())
	})
}
