package providers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer создаёт httptest.Server, который отвечает заданным JSON-телом
// на любой GET-запрос. Возвращает URL сервера, который нужно присвоить
// в поле baseURL тестируемого клиента.
//
// Используется в тестах в этом пакете: поле baseURL приватное, но тесты
// здесь же и могут его подменить перед вызовом.
func newTestServer(t *testing.T, jsonBody string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, jsonBody)
	}))
	t.Cleanup(srv.Close)

	return srv
}

// newStatusServer создаёт httptest.Server, который всегда отвечает
// заданным HTTP-статусом и пустым телом.
func newStatusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return srv
}
