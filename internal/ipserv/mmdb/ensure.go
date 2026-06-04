package mmdb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
	"github.com/denisdubovitskiy/vpnconfig/internal/logger"
)

const (
	// dirPerm — права на создаваемые промежуточные директории для MMDB-файла.
	dirPerm os.FileMode = 0o755
	// tmpSuffix — суффикс временного файла при атомарной записи.
	tmpSuffix = ".tmp"
)

// ensureDatabase проверяет наличие MMDB-файла на диске. Если файл отсутствует,
// имеет нулевой размер или старше cfg.MaxAge — скачивает его заново по
// cfg.EffectiveDownloadURL(). Скачивание выполняется атомарно (через временный
// файл + rename) с созданием всех промежуточных директорий в пути.
// Логгер берётся из ctx через logger.FromContext.
func ensureDatabase(
	ctx context.Context,
	cfg *config.MMDBConfig,
	client HTTPDoer,
) error {
	log := logger.FromContext(ctx)

	info, err := os.Stat(cfg.DatabasePath)
	needDownload := false

	switch {
	case err != nil && !os.IsNotExist(err):
		return fmt.Errorf("stat mmdb file: %w", err)
	case err == nil && info.Size() > 0:
		if isStale(info, cfg.MaxAge.Duration) {
			log.Info(
				"mmdb database is stale, re-downloading",
				"path", cfg.DatabasePath,
				"age", time.Since(info.ModTime()).Round(time.Second).String(),
				"max_age", cfg.MaxAge.String(),
			)
			if rmErr := os.Remove(cfg.DatabasePath); rmErr != nil && !os.IsNotExist(rmErr) {
				return fmt.Errorf("remove stale mmdb file: %w", rmErr)
			}
			needDownload = true
		}
	default:
		needDownload = true
	}

	if !needDownload {
		return nil
	}

	downloadURL := cfg.EffectiveDownloadURL()
	log.Info(
		"mmdb database not found locally, downloading",
		"path", cfg.DatabasePath,
		"url", downloadURL,
	)

	if err := downloadFile(ctx, downloadURL, cfg.DatabasePath, client); err != nil {
		return fmt.Errorf("download mmdb: %w", err)
	}

	log.Info("mmdb database downloaded", "path", cfg.DatabasePath)
	return nil
}

// isStale возвращает true, если файл старше maxAge.
// maxAge == 0 означает "проверка возраста отключена" — файл всегда свежий.
func isStale(info os.FileInfo, maxAge time.Duration) bool {
	return maxAge > 0 && time.Since(info.ModTime()) > maxAge
}

// downloadFile скачивает файл по url в destPath. Создаёт все промежуточные
// директории. Скачивание атомарно: данные пишутся во временный файл,
// который переименовывается в destPath при успешной записи.
func downloadFile(
	ctx context.Context,
	url, destPath string,
	client HTTPDoer,
) error {
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create mmdb directory %q: %w", dir, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	//nolint:errcheck // стандартная идиома игнорировать ошибку Close body.
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	return atomicWriteFile(destPath, resp.Body)
}

// atomicWriteFile пишет содержимое body в destPath атомарно: данные сначала
// попадают во временный файл (destPath + tmpSuffix), и только при успешной
// записи временный файл переименовывается в destPath. При любой ошибке
// временный файл удаляется, целевой файл не создаётся.
func atomicWriteFile(destPath string, body io.Reader) (err error) {
	tmpPath := destPath + tmpSuffix
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	defer func() {
		// Закрываем файл. Если к этому моменту уже есть другая ошибка —
		// игнорируем ошибку Close. Иначе — пробрасываем её как результат.
		if cerr := f.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("close temp file: %w", cerr)
		}
		// При любой ошибке удаляем временный файл, чтобы не оставлять мусор.
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = io.Copy(f, body); err != nil {
		return fmt.Errorf("copy body: %w", err)
	}
	if err = os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
