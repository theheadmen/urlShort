package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/theheadmen/urlShort/cmd/logger"
	"github.com/theheadmen/urlShort/cmd/models"
	config "github.com/theheadmen/urlShort/cmd/serverconfig"
	"github.com/theheadmen/urlShort/cmd/storager"
	"go.uber.org/zap"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

type ServerDataStore struct {
	configStore config.ConfigStore
	storager    *storager.Storager
}

func NewServerDataStore(configStore *config.ConfigStore, storager *storager.Storager) *ServerDataStore {
	return &ServerDataStore{
		configStore: *configStore,
		storager:    storager,
	}
}

func main() {
	configStore := config.NewConfigStore()
	configStore.ParseFlags()

	if err := logger.Initialize(configStore.FlagLogLevel); err != nil {
		panic(err)
	}
	logger.Log.Info("Running server", zap.String("address", configStore.FlagRunAddr), zap.String("short address", configStore.FlagShortRunAddr), zap.String("file", configStore.FlagFile))

	err := http.ListenAndServe(configStore.FlagRunAddr, makeChiServ(configStore, true /*isWithFile*/))
	if err != nil {
		logger.Log.Fatal("Server is down", zap.String("address", err.Error()))
		panic(err)
	}
}

func makeChiServ(configStore *config.ConfigStore, isWithFile bool) chi.Router {
	storager := storager.NewStorager(configStore.FlagFile, isWithFile, make(map[string]string))
	storager.ReadAllDataFromFile()

	dataStore := NewServerDataStore(configStore, storager)
	router := chi.NewRouter()

	// Add gzip middleware
	router.Use(middleware.Compress(5, "text/html", "application/json"))
	// Add the logger middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Log.Info("Request processed",
				zap.String("method", r.Method),
				zap.String("uri", r.RequestURI),
				zap.Duration("duration", time.Since(start)),
				zap.Int("status", ww.Status()),
				zap.Int("size", ww.BytesWritten()),
			)
		})
	})

	router.Get("/", dataStore.getHandler)
	router.Get("/{shortUrl}", dataStore.getHandler)
	router.Post("/", dataStore.postHandler)
	router.Post("/api/shorten", dataStore.postJSONHandler)
	return router
}

func (dataStore *ServerDataStore) postHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Log.Debug("cannot read request body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	url := string(body)
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(strings.NewReader(string(body)))
		if err != nil {
			logger.Log.Debug("cannot decompress request body", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		decompressed, err := io.ReadAll(gz)
		if err != nil {
			logger.Log.Debug("cannot read decompressed request body", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		url = string(decompressed)
	}

	shortURL := generateShortURL(url)

	dataStore.storager.StorageURL(shortURL, url)

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusCreated)
	servShortURL := ""
	// так как в тестах мы не используем флаги, нужно обезопасить себя
	if dataStore.configStore.FlagShortRunAddr == "" {
		servShortURL = "http://localhost:8080"
	} else {
		servShortURL = dataStore.configStore.FlagShortRunAddr
	}

	logger.Log.Info("After POST request", zap.String("body", url), zap.String("result", servShortURL+"/"+shortURL), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	fmt.Fprintf(w, servShortURL+"/%s", shortURL)
}

func (dataStore *ServerDataStore) postJSONHandler(w http.ResponseWriter, r *http.Request) {
	var req models.Request
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	if req.URL == "" {
		logger.Log.Debug("after decoding JSON we don't have any URL")
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	shortURL := generateShortURL(req.URL)

	dataStore.storager.StorageURL(shortURL, req.URL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	servShortURL := ""
	// так как в тестах мы не используем флаги, нужно обезопасить себя
	if dataStore.configStore.FlagShortRunAddr == "" {
		servShortURL = "http://localhost:8080"
	} else {
		servShortURL = dataStore.configStore.FlagShortRunAddr
	}

	// заполняем модель ответа
	resp := models.Response{
		Result: servShortURL + "/" + shortURL,
	}

	logger.Log.Info("After POST JSON request", zap.String("body", req.URL), zap.String("result", servShortURL+"/"+shortURL), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		logger.Log.Debug("error encoding response", zap.Error(err))
		return
	}
}

func (dataStore *ServerDataStore) getHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")
	originalURL, ok := dataStore.storager.GetURL(id)

	if !ok {
		logger.Log.Debug("cannot find url by id", zap.String("id", id))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logger.Log.Info("After GET request", zap.String("id", id), zap.String("originalURL", originalURL))

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func generateShortURL(url string) string {
	hash := sha256.Sum256([]byte(url))
	encoded := base64.URLEncoding.EncodeToString(hash[:])
	return encoded[:8]
}
