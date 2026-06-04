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

	// Проверяем ошибку записи при невалидном пути.
	t.Run("error on invalid path", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{}
		invalidPath := filepath.Join(t.TempDir(), "nonexistent_subdir", "singbox.json")

		// act
		err := SaveConfig(invalidPath, cfg)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write singbox config")
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

	// Защита от регрессии: GenerateSectionOutbounds не должен мутировать
	// входные proxy outbounds. Это критично, когда один и тот же Outbound
	// попадает в несколько секций (например, Россия в MULTI_WEST и MULTI_RU):
	// без копии тег первой секции перезаписывался тегом второй, и в
	// сохранённом sing-box.json появлялись перепутанные/дублирующиеся теги.
	t.Run("does not mutate input proxies", func(t *testing.T) {
		t.Parallel()

		// arrange
		proxies := []Outbound{
			{"type": "vless", "server": "203.0.113.1"},
			{"type": "trojan", "server": "203.0.113.2"},
		}

		// act
		_ = GenerateSectionOutbounds("SECTION_A", proxies, "https://test.com", "3m", 50)
		_ = GenerateSectionOutbounds("SECTION_B", proxies, "https://test.com", "3m", 50)

		// assert: теги и остальные поля входных мап не изменились.
		assert.Empty(t, proxies[0].Tag(), "first proxy tag must remain empty after generation")
		assert.Empty(t, proxies[1].Tag(), "second proxy tag must remain empty after generation")
		assert.Equal(t, "203.0.113.1", proxies[0]["server"])
		assert.Equal(t, "203.0.113.2", proxies[1]["server"])
	})

	// Защита от регрессии: повторный вызов с тем же input даёт независимые
	// результаты — сгенерированные outbounds не должны разделять общие мапы,
	// иначе изменение тега в одной секции повлияет на другую.
	t.Run("results are independent across calls", func(t *testing.T) {
		t.Parallel()

		// arrange
		proxies := []Outbound{
			{"type": "vless", "server": "203.0.113.1"},
		}

		// act
		first := GenerateSectionOutbounds("SECTION_A", proxies, "https://test.com", "3m", 50)
		second := GenerateSectionOutbounds("SECTION_B", proxies, "https://test.com", "3m", 50)

		// assert: теги в обоих результатах соответствуют своим секциям.
		assert.Equal(t, "SECTION_A-1-out", first[0].Tag())
		assert.Equal(t, "SECTION_B-1-out", second[0].Tag())
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

func TestAddOutbounds(t *testing.T) {
	t.Parallel()

	// Проверяем добавление outbounds в пустой список.
	t.Run("appends to empty list", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{}

		// act
		cfg.AddOutbounds([]Outbound{{"type": "vless", "tag": "vless-1-out"}})

		// assert
		require.Len(t, cfg.Outbounds, 1)
		assert.Equal(t, "vless-1-out", cfg.Outbounds[0].Tag())
	})

	// Проверяем добавление к существующему списку.
	t.Run("appends to existing list", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{
			Outbounds: []Outbound{{"type": "direct", "tag": "direct-out"}},
		}

		// act
		cfg.AddOutbounds([]Outbound{
			{"type": "vless", "tag": "vless-1-out"},
			{"type": "trojan", "tag": "trojan-1-out"},
		})

		// assert
		require.Len(t, cfg.Outbounds, 3)
		assert.Equal(t, "direct-out", cfg.Outbounds[0].Tag())
		assert.Equal(t, "vless-1-out", cfg.Outbounds[1].Tag())
		assert.Equal(t, "trojan-1-out", cfg.Outbounds[2].Tag())
	})
}

func TestCloneOutbounds(t *testing.T) {
	t.Parallel()

	// Проверяем клонирование непустого списка.
	t.Run("clones non-empty list", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{
			Outbounds: []Outbound{
				{"type": "vless", "tag": "test-1-out"},
				{"type": "trojan", "tag": "test-2-out"},
			},
		}

		// act
		clone, err := cfg.CloneOutbounds()

		// assert
		require.NoError(t, err)
		require.Len(t, clone, 2)
		assert.Equal(t, "test-1-out", clone[0].Tag())
		assert.Equal(t, "test-2-out", clone[1].Tag())
	})

	// Проверяем возврат nil для пустого списка.
	t.Run("empty config returns nil", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{}

		// act
		clone, err := cfg.CloneOutbounds()

		// assert
		require.NoError(t, err)
		assert.Nil(t, clone)
	})

	// Проверяем, что клон — глубокая копия.
	t.Run("produces deep copy", func(t *testing.T) {
		t.Parallel()

		// arrange
		cfg := &Config{
			Outbounds: []Outbound{
				{"type": "vless", "tag": "original", "server": "1.1.1.1"},
			},
		}

		// act
		clone, err := cfg.CloneOutbounds()
		require.NoError(t, err)
		clone[0].SetTag("modified")
		clone[0]["server"] = "2.2.2.2"

		// assert
		assert.Equal(t, "original", cfg.Outbounds[0].Tag())
		assert.Equal(t, "1.1.1.1", cfg.Outbounds[0]["server"])
	})
}

func TestConvertFromSingBoxOutbound(t *testing.T) {
	t.Parallel()

	// Проверяем конвертацию из map.
	t.Run("converts from map", func(t *testing.T) {
		t.Parallel()

		// arrange
		src := map[string]any{
			"type":   "vless",
			"tag":    "test-out",
			"server": "1.1.1.1",
		}

		// act
		out, err := ConvertFromSingBoxOutbound(src)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "vless", out.Type())
		assert.Equal(t, "test-out", out.Tag())
		assert.Equal(t, "1.1.1.1", out["server"])
	})
}

func TestOutbound_Type(t *testing.T) {
	t.Parallel()

	// Проверяем получение типа как строки.
	t.Run("returns type for string", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := Outbound{"type": "vless"}

		// act & assert
		assert.Equal(t, "vless", o.Type())
	})

	// Проверяем возврат пустой строки при отсутствии типа.
	t.Run("returns empty for missing type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := Outbound{}

		// act & assert
		assert.Empty(t, o.Type())
	})

	// Проверяем возврат пустой строки для не-строкового типа.
	t.Run("returns empty for non-string type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := Outbound{"type": 42}

		// act & assert
		assert.Empty(t, o.Type())
	})
}

func TestStore_LoadConfig(t *testing.T) {
	t.Parallel()

	// Проверяем делегирование в LoadConfig.
	t.Run("delegates to LoadConfig", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "singbox.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"outbounds":[]}`), 0o644))

		// act
		cfg, err := (&Store{}).LoadConfig(path)

		// assert
		require.NoError(t, err)
		require.NotNil(t, cfg)
	})
}

func TestStore_SaveConfig(t *testing.T) {
	t.Parallel()

	// Проверяем делегирование в SaveConfig.
	t.Run("delegates to SaveConfig", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "singbox.json")
		cfg := &Config{Outbounds: []Outbound{{"type": "direct", "tag": "direct-out"}}}
		store := &Store{}

		// act
		err := store.SaveConfig(path, cfg)

		// assert
		require.NoError(t, err)

		loaded, err := LoadConfig(path)
		require.NoError(t, err)
		require.Len(t, loaded.Outbounds, 1)
		assert.Equal(t, "direct-out", loaded.Outbounds[0].Tag())
	})
}

func TestStore_CreateBackup(t *testing.T) {
	t.Parallel()

	// Проверяем делегирование в CreateBackup.
	t.Run("delegates to CreateBackup", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "singbox.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"outbounds":[]}`), 0o644))

		// act
		backupPath, err := (&Store{}).CreateBackup(path)

		// assert
		require.NoError(t, err)
		assert.Contains(t, backupPath, ".backup_")

		original, err := os.ReadFile(path)
		require.NoError(t, err)
		backed, err := os.ReadFile(backupPath)
		require.NoError(t, err)
		assert.Equal(t, original, backed)
	})
}
