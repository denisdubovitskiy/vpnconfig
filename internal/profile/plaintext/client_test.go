package plaintext

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestClient_FetchLinks(t *testing.T) {
	t.Parallel()

	const testURL = "https://example.com/links.txt"

	// Проверяем успешный запрос и парсинг ссылок.
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockDoer := NewMockDoer(t)
		client := NewClient(mockDoer)

		content := "vless://link1\nvmess://link2\n\n\ntrojan://link3\n"

		mockDoer.EXPECT().
			Do(mock.Anything).
			Run(func(req *http.Request) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, testURL, req.URL.String())
			}).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(content)),
				},
				nil,
			)

		// act
		links, err := client.FetchLinks(t.Context(), testURL)

		// assert
		require.NoError(t, err)
		assert.Equal(
			t,
			[]string{
				"vless://link1",
				"vmess://link2",
				"trojan://link3",
			},
			links,
		)
	})

	// Проверяем обработку ошибки HTTP.
	t.Run("http error", func(t *testing.T) {
		t.Parallel()

		// arrange
		wantErr := errors.New("connection refused")
		mockDoer := NewMockDoer(t)
		client := NewClient(mockDoer)

		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(nil, wantErr)

		// act
		_, err := client.FetchLinks(t.Context(), testURL)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)
		assert.Contains(t, err.Error(), "execute request")
	})

	// Проверяем обработку невалидного HTTP-статуса.
	t.Run("non-ok status code", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockDoer := NewMockDoer(t)
		client := NewClient(mockDoer)

		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("")),
				},
				nil,
			)

		// act
		_, err := client.FetchLinks(t.Context(), testURL)

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 404")
	})

	// Проверяем обработку пустого ответа.
	t.Run("empty response", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockDoer := NewMockDoer(t)
		client := NewClient(mockDoer)

		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
				},
				nil,
			)

		// act
		links, err := client.FetchLinks(t.Context(), testURL)

		// assert
		require.NoError(t, err)
		assert.Empty(t, links)
	})

	// Проверяем фильтрацию пустых строк.
	t.Run("filters blank lines", func(t *testing.T) {
		t.Parallel()

		// arrange
		mockDoer := NewMockDoer(t)
		client := NewClient(mockDoer)

		content := "\n\nvless://link1\n  \n\nvless://link2\n\n"

		mockDoer.EXPECT().
			Do(mock.Anything).
			Return(
				&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(content)),
				},
				nil,
			)

		// act
		links, err := client.FetchLinks(t.Context(), testURL)

		// assert
		require.NoError(t, err)
		assert.Equal(t, []string{"vless://link1", "vless://link2"}, links)
	})
}
