package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	// Проверяем успешную загрузку конфига.
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		data := []byte(`
happ_url: "https://example.com/api"
cache_path: "/tmp/cache.json"
sections:
  - name: "Europe"
    countries:
      - "Germany"
      - "France"
`)
		err := os.WriteFile(path, data, 0o644)
		require.NoError(t, err)

		// act
		cfg, err := Load(path)

		// assert
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/api", cfg.HappURL)
		assert.Equal(t, "/tmp/cache.json", cfg.CachePath)
		require.Len(t, cfg.Sections, 1)
		assert.Equal(t, "Europe", cfg.Sections[0].Name)
		assert.Equal(t, []string{"Germany", "France"}, cfg.Sections[0].Countries)
	})

	// Проверяем ошибку при отсутствии файла.
	t.Run("file not found", func(t *testing.T) {
		t.Parallel()

		// act
		_, err := Load("/nonexistent/config.yaml")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read config file")
	})

	// Проверяем ошибку при невалидном YAML.
	t.Run("invalid yaml", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		err := os.WriteFile(path, []byte("not: valid: yaml: ["), 0o644)
		require.NoError(t, err)

		// act
		_, err = Load(path)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse config")
	})
}
