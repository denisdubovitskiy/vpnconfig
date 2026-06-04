package providers

import "net/http"

// Doer выполняет HTTP-запросы.
//
// Это общий контракт для всех провайдеров пакета: каждый провайдер
// принимает Doer в конструкторе, чтобы его можно было подменить
// в тестах через httptest.Server или mock.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}
