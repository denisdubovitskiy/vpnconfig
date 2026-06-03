package singbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBackup(t *testing.T) {
	t.Parallel()

	// Проверяем успешное создание backup.
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		configPath := filepath.Join(dir, "singbox.json")
		originalData := []byte(`{"outbounds":[{"type":"direct","tag":"direct-out"}]}`)
		err := os.WriteFile(configPath, originalData, 0o644)
		require.NoError(t, err)

		// act
		backupPath, err := CreateBackup(configPath)

		// assert
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(backupPath, configPath+".backup_"))

		backupData, err := os.ReadFile(backupPath)
		require.NoError(t, err)
		assert.Equal(t, originalData, backupData)
	})

	// Проверяем ошибку при отсутствии исходного файла.
	t.Run("file not found", func(t *testing.T) {
		t.Parallel()

		// act
		_, err := CreateBackup("/nonexistent/singbox.json")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read config for backup")
	})
}
