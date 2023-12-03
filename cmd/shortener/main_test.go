package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRequest(t *testing.T, ts *httptest.Server, method, path string, body io.Reader) (*http.Response, string) {
	req, err := http.NewRequest(method, ts.URL+path, body)
	require.NoError(t, err)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

func TestSimpleHandler(t *testing.T) {
	ts := httptest.NewServer(makeChiServ())
	defer ts.Close()

	testCases := []struct {
		method       string
		testValue    string
		testUrl      string
		expectedCode int
		expectedBody string
	}{
		{method: http.MethodGet, testValue: "", testUrl: "", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodGet, testValue: "", testUrl: "1MnZAnMm", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodPut, testValue: "", testUrl: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodDelete, testValue: "", testUrl: "", expectedCode: http.StatusMethodNotAllowed, expectedBody: ""},
		{method: http.MethodPost, testValue: "", testUrl: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/47DEQpj8"},
		{method: http.MethodPost, testValue: "google.com", testUrl: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/1MnZAnMm"},
		{method: http.MethodPost, testValue: "yandex.ru", testUrl: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/eeILJFID"},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			body := strings.NewReader(tc.testValue)
			resp, get := testRequest(t, ts, tc.method, "/"+tc.testUrl, body)

			assert.Equal(t, tc.expectedCode, resp.StatusCode, "Код ответа не совпадает с ожидаемым")
			if tc.expectedBody != "" {
				assert.Equal(t, tc.expectedBody, get, "Тело ответа не совпадает с ожидаемым")
			}
		})
	}
}

func TestSequenceHandler(t *testing.T) {
	testCases := []struct {
		testUrl          string
		expectedShortUrl string
		returnCode       int
	}{
		{testUrl: "google.com", expectedShortUrl: "1MnZAnMm", returnCode: http.StatusTemporaryRedirect},
		{testUrl: "google.com", expectedShortUrl: "1MnZm", returnCode: http.StatusBadRequest},
		{testUrl: "yandex.ru", expectedShortUrl: "eeILJFID", returnCode: http.StatusTemporaryRedirect},
		{testUrl: "yandex.ru", expectedShortUrl: "eeFID", returnCode: http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.testUrl, func(t *testing.T) {
			// тестим последовательно пост + гет запросы
			body := strings.NewReader(tc.testUrl)
			req1 := httptest.NewRequest("POST", "/", body)

			req2 := httptest.NewRequest("GET", "/"+tc.expectedShortUrl, nil)

			// для этого используем два рекордера, по одному для каждого запроса
			recorder1 := httptest.NewRecorder()
			recorder2 := httptest.NewRecorder()
			handlerFunc := http.HandlerFunc(postHandler)
			handlerFunc.ServeHTTP(recorder1, req1)
			handlerFunc2 := http.HandlerFunc(getHandler)
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

				location := recorder2.HeaderMap.Get("Location")
				if location != tc.testUrl {
					t.Errorf("обработчик вернул неожиданный заголовок Location: получили %v хотели %v", location, tc.testUrl)
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
			assert.Equal(t, generateShortURL(test.value), test.want)
		})
	}
}
