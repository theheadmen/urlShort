package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/serverapi"
	config "github.com/theheadmen/urlShort/internal/serverconfig"
	"github.com/theheadmen/urlShort/internal/storage"
	"github.com/theheadmen/urlShort/internal/storage/file"
)

func NewTestConfigStore() *config.ConfigStore {
	return &config.ConfigStore{
		FlagRunAddr:      ":8080",
		FlagShortRunAddr: "http://localhost:8080",
		FlagLogLevel:     "debug",
		FlagFile:         "/tmp/short-url-db.json",
		FlagDB:           "",
	}
}

func testRequest(t *testing.T, ts *httptest.Server, method, path string, bodyValue io.Reader, cookie *http.Cookie) (*http.Response, string) {
	req, err := http.NewRequest(method, ts.URL+path, bodyValue)
	require.NoError(t, err)

	if cookie != nil {
		req.AddCookie(cookie)
	}

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		require.NoError(t, err)

		respBody, err := io.ReadAll(gz)
		require.NoError(t, err)

		return resp, string(respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

func TestSimpleHandler(t *testing.T) {
	configStore := NewTestConfigStore()
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	ts := httptest.NewServer(serverapi.MakeChiServ(configStore, storager))
	defer ts.Close()

	testCases := []struct {
		method       string
		testValue    string
		testURL      string
		expectedCode int
		expectedBody string
	}{
		{method: http.MethodGet, testValue: "", testURL: "", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodGet, testValue: "", testURL: "1MnZAnMm", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodPut, testValue: "", testURL: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodDelete, testValue: "", testURL: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPost, testValue: "", testURL: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/47DEQpj8"},
		{method: http.MethodPost, testValue: "google.com", testURL: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/1MnZAnMm"},
		{method: http.MethodPost, testValue: "yandex.ru", testURL: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/eeILJFID"},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			testValue := strings.NewReader(tc.testValue)
			resp, get := testRequest(t, ts, tc.method, "/"+tc.testURL, testValue, nil)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedCode, resp.StatusCode, "Код ответа не совпадает с ожидаемым")
			if tc.expectedBody != "" {
				assert.Equal(t, tc.expectedBody, get, "Тело ответа не совпадает с ожидаемым")
			}
		})
	}
}

func TestJsonPost(t *testing.T) {
	configStore := NewTestConfigStore()

	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	ts := httptest.NewServer(serverapi.MakeChiServ(configStore, storager))
	defer ts.Close()

	testCases := []struct {
		name         string // добавляем название тестов
		method       string
		body         string // добавляем тело запроса в табличные тесты
		expectedCode int
		expectedBody string
	}{
		{
			name:         "method_get",
			method:       http.MethodGet,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_put",
			method:       http.MethodPut,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_delete",
			method:       http.MethodDelete,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_post_without_body",
			method:       http.MethodPost,
			expectedCode: http.StatusUnprocessableEntity,
			expectedBody: "",
		},
		{
			name:         "method_post_unsupported_type",
			method:       http.MethodPost,
			body:         `{"request": {"type": "idunno", "command": "do something"}, "version": "1.0"}`,
			expectedCode: http.StatusUnprocessableEntity,
			expectedBody: "",
		},
		{
			name:         "method_post_success",
			method:       http.MethodPost,
			body:         `{"url": "google.com"}`,
			expectedCode: http.StatusCreated,
			expectedBody: `{"result":"http://localhost:8080/1MnZAnMm"}`,
		}, {
			name:         "method_post_success",
			method:       http.MethodPost,
			body:         `{"url": "yandex.ru"}`,
			expectedCode: http.StatusCreated,
			expectedBody: `{"result":"http://localhost:8080/eeILJFID"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			testValue := strings.NewReader(tc.body)
			resp, get := testRequest(t, ts, tc.method, "/api/shorten", testValue, nil)
			get = strings.TrimSuffix(string(get), "\n")
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedCode, resp.StatusCode, "Код ответа не совпадает с ожидаемым")
			if tc.expectedBody != "" {
				assert.Equal(t, tc.expectedBody, get, "Тело ответа не совпадает с ожидаемым")
			}
		})
	}
}

func TestJsonBatchPost(t *testing.T) {
	configStore := NewTestConfigStore()

	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	ts := httptest.NewServer(serverapi.MakeChiServ(configStore, storager))
	defer ts.Close()

	testCases := []struct {
		name         string // добавляем название тестов
		method       string
		body         string // добавляем тело запроса в табличные тесты
		expectedCode int
		expectedBody string
	}{
		{
			name:         "method_get",
			method:       http.MethodGet,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_put",
			method:       http.MethodPut,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_delete",
			method:       http.MethodDelete,
			expectedCode: http.StatusMethodNotAllowed,
			expectedBody: "",
		},
		{
			name:         "method_post_without_body",
			method:       http.MethodPost,
			expectedCode: http.StatusUnprocessableEntity,
			expectedBody: "",
		},
		{
			name:         "method_post_unsupported_type",
			method:       http.MethodPost,
			body:         `{"request": {"type": "idunno", "command": "do something"}, "version": "1.0"}`,
			expectedCode: http.StatusUnprocessableEntity,
			expectedBody: "",
		},
		{
			name:         "method_post_success",
			method:       http.MethodPost,
			body:         `[{"correlation_id":"u1","original_url":"google.com"},{"correlation_id":"u2","original_url":"ya.ru"}]`,
			expectedCode: http.StatusCreated,
			expectedBody: `[{"correlation_id":"u1","short_url":"http://localhost:8080/1MnZAnMm"},{"correlation_id":"u2","short_url":"http://localhost:8080/fE54KN4v"}]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			testValue := strings.NewReader(tc.body)
			resp, get := testRequest(t, ts, tc.method, "/api/shorten/batch", testValue, nil)
			get = strings.TrimSuffix(string(get), "\n")
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedCode, resp.StatusCode, "Код ответа не совпадает с ожидаемым")
			if tc.expectedBody != "" {
				assert.Equal(t, tc.expectedBody, get, "Тело ответа не совпадает с ожидаемым")
			}
		})
	}
}

func TestSequenceHandler(t *testing.T) {
	configStore := NewTestConfigStore()

	testCases := []struct {
		testURL          string
		expectedShortURL string
		returnCode       int
	}{
		{testURL: "google.com", expectedShortURL: "1MnZAnMm", returnCode: http.StatusTemporaryRedirect},
		{testURL: "google.com", expectedShortURL: "1MnZm", returnCode: http.StatusBadRequest},
		{testURL: "yandex.ru", expectedShortURL: "eeILJFID", returnCode: http.StatusTemporaryRedirect},
		{testURL: "yandex.ru", expectedShortURL: "eeFID", returnCode: http.StatusBadRequest},
		{testURL: "http://mct5yhzz7q.yandex/ablfpjxrq", expectedShortURL: "QU5zXC-Z", returnCode: http.StatusTemporaryRedirect},
	}

	for _, tc := range testCases {
		t.Run(tc.testURL, func(t *testing.T) {
			storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
			dataStore := serverapi.NewServerDataStore(configStore, storager)
			// тестим последовательно пост + гет запросы
			body := strings.NewReader(tc.testURL)

			req1 := httptest.NewRequest("POST", "/", body)
			req1.AddCookie(serverapi.GetTestCookie())
			req2 := httptest.NewRequest("GET", "/"+tc.expectedShortURL, nil)
			req2.AddCookie(serverapi.GetTestCookie())

			// для этого используем два рекордера, по одному для каждого запроса
			recorder1 := httptest.NewRecorder()
			recorder2 := httptest.NewRecorder()

			handlerFunc := http.HandlerFunc(dataStore.PostHandler)
			handlerFunc.ServeHTTP(recorder1, req1)

			handlerFunc2 := http.HandlerFunc(dataStore.GetHandler)
			handlerFunc2.ServeHTTP(recorder2, req2)

			// сначала проверка что post сработал
			if status := recorder1.Code; status != http.StatusCreated {
				t.Errorf("обработчик вернул неверный код состояния: получили %v хотели %v", status, http.StatusCreated)
			}

			// затем мы или проверяем что в Location
			if tc.returnCode == http.StatusTemporaryRedirect {
				if status := recorder2.Code; status != tc.returnCode {
					t.Errorf("обработчик вернул неверный код состояния: получили %v хотели %v", status, tc.returnCode)
				}

				location := recorder2.Header().Get("Location")
				if location != tc.testURL {
					t.Errorf("обработчик вернул неожиданный заголовок Location: получили %v хотели %v", location, tc.testURL)
				}
			} else { // или проверяем что на неверный код будет ошибка
				if status := recorder2.Code; status != tc.returnCode {
					t.Errorf("обработчик вернул неверный код состояния: получили %v хотели %v", status, tc.returnCode)
				}
			}
		})
	}
}

func TestGenerateShortURL(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "simple test #1",
			value: "google.com",
			want:  "1MnZAnMm",
		},
		{
			name:  "simple test #2",
			value: "ya.ru",
			want:  "fE54KN4v",
		},
		{
			name:  "simple test #3",
			value: "yandex.ru",
			want:  "eeILJFID",
		},
		{
			name:  "simple test #4",
			value: "",
			want:  "47DEQpj8",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, serverapi.GenerateShortURL(test.value), test.want)
		})
	}
}

func TestCompressResponse(t *testing.T) {
	configStore := NewTestConfigStore()

	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	dataStore := serverapi.NewServerDataStore(configStore, storager)
	r := chi.NewRouter()

	r.Use(middleware.Compress(5, "text/html", "application/json"))
	r.Post("/", dataStore.PostHandler)

	t.Run("with Accept-Encoding", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("google.com"))
		req.AddCookie(serverapi.GetTestCookie())
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		body, _ := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"), "Не тот тип кодирования контента")

		gz, err := gzip.NewReader(strings.NewReader(string(body)))
		require.NoError(t, err)
		defer gz.Close()

		decompressed, err := io.ReadAll(gz)
		require.NoError(t, err)

		assert.Equal(t, "http://localhost:8080/1MnZAnMm", string(decompressed), "Тело ответа не совпадает с ожидаемым")
	})

	t.Run("without Accept-Encoding", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("google.com"))
		req.AddCookie(serverapi.GetTestCookie())
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		body, _ := io.ReadAll(resp.Body)
		defer resp.Body.Close()

		assert.Equal(t, "", resp.Header.Get("Content-Encoding"), "Не тот тип кодирования контента")

		assert.Equal(t, "http://localhost:8080/1MnZAnMm", string(body), "Тело ответа не совпадает с ожидаемым")
	})
}
