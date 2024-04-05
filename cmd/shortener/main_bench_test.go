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
	"github.com/stretchr/testify/require"
	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/serverapi"
	"github.com/theheadmen/urlShort/internal/storage"
	"github.com/theheadmen/urlShort/internal/storage/file"
)

func testBRequest(t *testing.B, ts *httptest.Server, method, path string, bodyValue io.Reader, cookie *http.Cookie) (*http.Response, string) {
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

func BenchmarkSimpleHandler(b *testing.B) {
	b.ReportAllocs()
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
		{method: http.MethodPost, testValue: "", testURL: "", expectedCode: http.StatusCreated, expectedBody: "http://localhost:8080/47DEQpj8"},
	}

	b.ResetTimer() // Reset the timer after the setup is done

	for i := 0; i < b.N; i++ {
		// Choose a test case that you want to benchmark
		tc := testCases[0] // For example, the first test case

		testValue := strings.NewReader(tc.testValue)
		resp, _ := testBRequest(b, ts, tc.method, "/"+tc.testURL, testValue, nil)
		resp.Body.Close()
	}
}

func BenchmarkTestJsonPost(t *testing.B) {
	t.ReportAllocs()
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
			name:         "method_post_success",
			method:       http.MethodPost,
			body:         `{"url": "yandex.ru"}`,
			expectedCode: http.StatusCreated,
			expectedBody: `{"result":"http://localhost:8080/eeILJFID"}`,
		},
	}
	tc := testCases[0]

	t.ResetTimer() // Reset the timer after the setup is done

	for i := 0; i < t.N; i++ {
		testValue := strings.NewReader(tc.body)
		resp, get := testBRequest(t, ts, tc.method, "/api/shorten", testValue, nil)
		strings.TrimSuffix(string(get), "\n")
		resp.Body.Close()
	}
}

func BenchmarkTestJsonBatchPost(t *testing.B) {
	t.ReportAllocs()
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
			name:         "method_post_success",
			method:       http.MethodPost,
			body:         `[{"correlation_id":"u1","original_url":"google.com"},{"correlation_id":"u2","original_url":"ya.ru"}]`,
			expectedCode: http.StatusCreated,
			expectedBody: `[{"correlation_id":"u1","short_url":"http://localhost:8080/1MnZAnMm"},{"correlation_id":"u2","short_url":"http://localhost:8080/fE54KN4v"}]`,
		},
	}
	tc := testCases[0]
	t.ResetTimer() // Reset the timer after the setup is done

	for i := 0; i < t.N; i++ {
		testValue := strings.NewReader(tc.body)
		resp, get := testBRequest(t, ts, tc.method, "/api/shorten/batch", testValue, nil)
		strings.TrimSuffix(string(get), "\n")
		resp.Body.Close()
	}
}

func BenchmarkTestSequenceHandler(t *testing.B) {
	t.ReportAllocs()
	configStore := NewTestConfigStore()

	testCases := []struct {
		testURL          string
		expectedShortURL string
		returnCode       int
	}{
		{testURL: "google.com", expectedShortURL: "1MnZAnMm", returnCode: http.StatusTemporaryRedirect},
	}

	tc := testCases[0]
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	dataStore := serverapi.NewServerDataStore(configStore, storager)
	// тестим последовательно пост + гет запросы
	body := strings.NewReader(tc.testURL)

	t.ResetTimer() // Reset the timer after the setup is done

	for i := 0; i < t.N; i++ {
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
	}
}

func BenchmarkTestGenerateShortURL(t *testing.B) {
	t.ReportAllocs()
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
	}
	for i := 0; i < t.N; i++ {
		serverapi.GenerateShortURL(tests[0].value)
	}
}

func BenchmarkTestCompressAcceptResponse(t *testing.B) {
	t.ReportAllocs()
	configStore := NewTestConfigStore()
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	dataStore := serverapi.NewServerDataStore(configStore, storager)
	r := chi.NewRouter()
	r.Use(middleware.Compress(5, "text/html", "application/json"))
	r.Post("/", dataStore.PostHandler)

	t.ResetTimer() // Reset the timer after the setup is done

	for i := 0; i < t.N; i++ {
		req := httptest.NewRequest("POST", "/", strings.NewReader("google.com"))
		req.AddCookie(serverapi.GetTestCookie())
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		gz, _ := gzip.NewReader(strings.NewReader(string(body)))
		gz.Close()

		io.ReadAll(gz)
	}
}

func BenchmarkTestCompressWithoutAcceptResponse(t *testing.B) {
	t.ReportAllocs()
	configStore := NewTestConfigStore()
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	dataStore := serverapi.NewServerDataStore(configStore, storager)
	r := chi.NewRouter()
	r.Use(middleware.Compress(5, "text/html", "application/json"))
	r.Post("/", dataStore.PostHandler)

	t.ResetTimer() // Reset the timer after the setup is done

	for i := 0; i < t.N; i++ {
		req := httptest.NewRequest("POST", "/", strings.NewReader("google.com"))
		req.AddCookie(serverapi.GetTestCookie())
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}
}
