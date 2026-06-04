package mmdb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/vpnconfig/internal/config"
)

const (
	testIPGoogle     = "8.8.8.8"
	testIPCloudflare = "1.1.1.1"
	testIPPrivate    = "192.168.1.1"
	testIPPrivateRFC = "10.0.0.1"
	testIPInvalid    = "not-an-ip"
	testIPOtherLocal = "203.0.113.5"
)

const (
	testMMDBContent  = "mmdb-binary-data"
	testEmptyContent = ""
)

func newTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func newTestServerWithPayload(t *testing.T, payload string) *httptest.Server {
	t.Helper()
	return newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
}

func newMMDBConfig(databasePath, downloadURL string, opts ...mmdbConfigOpt) *config.MMDBConfig {
	cfg := &config.MMDBConfig{
		Enabled:      true,
		DatabasePath: databasePath,
		DownloadURL:  downloadURL,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

type mmdbConfigOpt func(*config.MMDBConfig)

func withMaxAge(d time.Duration) mmdbConfigOpt {
	return func(cfg *config.MMDBConfig) {
		cfg.MaxAge.Duration = d
	}
}

func writeStaleFile(t *testing.T, path, content string, age time.Duration) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	past := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(path, past, past))
}
