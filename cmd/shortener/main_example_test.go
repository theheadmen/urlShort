package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/serverapi"
	config "github.com/theheadmen/urlShort/internal/serverconfig"
	"github.com/theheadmen/urlShort/internal/storage"
	"github.com/theheadmen/urlShort/internal/storage/file"
)

func exampleConfigStore() *config.ConfigStore {
	return &config.ConfigStore{
		FlagRunAddr:      ":8080",
		FlagShortRunAddr: "http://localhost:8080",
		FlagLogLevel:     "debug",
		FlagFile:         "/tmp/short-url-db.json",
		FlagDB:           "",
	}
}

func testERequest(ts *httptest.Server, method, path string, bodyValue io.Reader, cookie *http.Cookie) (*http.Response, string) {
	req, _ := http.NewRequest(method, ts.URL+path, bodyValue)

	if cookie != nil {
		req.AddCookie(cookie)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		resp.Body.Close()
		return resp, ""
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, _ := gzip.NewReader(resp.Body)

		respBody, _ := io.ReadAll(gz)

		return resp, string(respBody)
	}

	respBody, _ := io.ReadAll(resp.Body)

	return resp, string(respBody)
}

func ExampleServerDataStore_PostHandler() {
	configStore := exampleConfigStore()
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false, make(map[storage.URLMapKey]models.SavedURL))
	ts := httptest.NewServer(serverapi.MakeChiServ(configStore, storager))
	defer ts.Close()

	testValue := strings.NewReader("")
	resp, get := testERequest(ts, http.MethodPost, "/", testValue, nil)
	defer resp.Body.Close()

	fmt.Println(resp.StatusCode)
	fmt.Println(get)

	// Output:
	// 201
	// http://localhost:8080/47DEQpj8
}

func ExampleServerDataStore_postJSONHandler() {
	configStore := NewTestConfigStore()

	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false, make(map[storage.URLMapKey]models.SavedURL))
	ts := httptest.NewServer(serverapi.MakeChiServ(configStore, storager))
	defer ts.Close()

	testValue := strings.NewReader(`{"url": "yandex.ru"}`)
	resp, get := testERequest(ts, http.MethodPost, "/api/shorten", testValue, nil)
	defer resp.Body.Close()

	fmt.Println(resp.StatusCode)
	fmt.Println(strings.TrimSuffix(string(get), "\n"))

	// Output:
	// 201
	// {"result":"http://localhost:8080/eeILJFID"}
}

func ExampleServerDataStore_postBatchJSONHandler() {
	configStore := NewTestConfigStore()

	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false, make(map[storage.URLMapKey]models.SavedURL))
	ts := httptest.NewServer(serverapi.MakeChiServ(configStore, storager))
	defer ts.Close()

	testValue := strings.NewReader(`[{"correlation_id":"u1","original_url":"google.com"},{"correlation_id":"u2","original_url":"ya.ru"}]`)
	resp, get := testERequest(ts, http.MethodPost, "/api/shorten/batch", testValue, nil)
	defer resp.Body.Close()

	fmt.Println(resp.StatusCode)
	fmt.Println(strings.TrimSuffix(string(get), "\n"))

	// Output:
	// 201
	// [{"correlation_id":"u1","short_url":"http://localhost:8080/1MnZAnMm"},{"correlation_id":"u2","short_url":"http://localhost:8080/fE54KN4v"}]
}

func ExampleServerDataStore_GetHandler() {
	configStore := NewTestConfigStore()

	testCases := []struct {
		testURL          string
		expectedShortURL string
		returnCode       int
	}{
		{testURL: "google.com", expectedShortURL: "1MnZAnMm", returnCode: http.StatusTemporaryRedirect},
	}

	tc := testCases[0]
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false, make(map[storage.URLMapKey]models.SavedURL))
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

	fmt.Println(recorder2.Code)
	fmt.Println(recorder2.Header().Get("Location"))

	// Output:
	// 307
	// google.com
}

func ExampleGenerateShortURL() {
	fmt.Println(serverapi.GenerateShortURL("google.com"))
	// Output:
	// 1MnZAnMm
}

func ExampleMakeChiServ() {
	configStore := NewTestConfigStore()
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false, make(map[storage.URLMapKey]models.SavedURL))
	dataStore := serverapi.NewServerDataStore(configStore, storager)
	r := chi.NewRouter()
	r.Use(middleware.Compress(5, "text/html", "application/json"))
	r.Post("/", dataStore.PostHandler)

	req := httptest.NewRequest("POST", "/", strings.NewReader("google.com"))
	req.AddCookie(serverapi.GetTestCookie())
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	gz, _ := gzip.NewReader(strings.NewReader(string(body)))
	gz.Close()
	resp.Body.Close()

	decompressed, _ := io.ReadAll(gz)
	fmt.Println(string(decompressed))

	// without compress

	req = httptest.NewRequest("POST", "/", strings.NewReader("google.com"))
	req.AddCookie(serverapi.GetTestCookie())
	w = httptest.NewRecorder()

	r.ServeHTTP(w, req)

	resp = w.Result()
	body2, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body2))
	resp.Body.Close()

	// Output:
	// http://localhost:8080/1MnZAnMm
	// http://localhost:8080/1MnZAnMm
}
