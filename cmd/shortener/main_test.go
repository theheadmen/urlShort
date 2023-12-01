package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleHandler(t *testing.T) {
	testCases := []struct {
		method       string
		testValue    string
		expectedCode int
		expectedBody string
	}{
		{method: http.MethodGet, testValue: "", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodGet, testValue: "47DEQpj8", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodPut, testValue: "", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodDelete, testValue: "", expectedCode: http.StatusBadRequest, expectedBody: ""},
		{method: http.MethodPost, testValue: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/47DEQpj8"},
		{method: http.MethodPost, testValue: "google.com", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/1MnZAnMm"},
		{method: http.MethodPost, testValue: "yandex.ru", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/eeILJFID"},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			body := strings.NewReader(tc.testValue)
			r := httptest.NewRequest(tc.method, "/", body)
			w := httptest.NewRecorder()

			handler(w, r)

			assert.Equal(t, tc.expectedCode, w.Code, "Код ответа не совпадает с ожидаемым")
			if tc.expectedBody != "" {
				assert.Equal(t, tc.expectedBody, w.Body.String(), "Тело ответа не совпадает с ожидаемым")
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
			handlerFunc := http.HandlerFunc(handler)
			handlerFunc.ServeHTTP(recorder1, req1)
			handlerFunc.ServeHTTP(recorder2, req2)

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
