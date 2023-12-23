package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/theheadmen/urlShort/cmd/logger"
	"github.com/theheadmen/urlShort/cmd/models"
	config "github.com/theheadmen/urlShort/cmd/serverconfig"
	"go.uber.org/zap"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

type ServerDataStore struct {
	urlMap      map[string]string
	mu          sync.RWMutex
	configStore config.ConfigStore
}

func NewServerDataStore(configStore *config.ConfigStore) *ServerDataStore {
	return &ServerDataStore{
		urlMap:      make(map[string]string),
		mu:          sync.RWMutex{},
		configStore: *configStore,
	}
}

func main() {
	configStore := config.NewConfigStore()
	configStore.ParseFlags()

	if err := logger.Initialize(configStore.FlagLogLevel); err != nil {
		panic(err)
	}
	logger.Log.Info("Running server", zap.String("address", configStore.FlagRunAddr))

	err := http.ListenAndServe(configStore.FlagRunAddr, makeChiServ(configStore))
	if err != nil {
		logger.Log.Fatal("Server is down", zap.String("address", err.Error()))
		panic(err)
	}
}

func makeChiServ(configStore *config.ConfigStore) chi.Router {
	dataStore := NewServerDataStore(configStore)
	router := chi.NewRouter()

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
	router.Post("/api/shorten", dataStore.postJsonHandler)
	return router
}

func (dataStore *ServerDataStore) postHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	url := string(body)
	shortURL := generateShortURL(url)

	dataStore.mu.Lock()
	dataStore.urlMap[shortURL] = url
	dataStore.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	servShortURL := ""
	// так как в тестах мы не используем флаги, нужно обезопасить себя
	if dataStore.configStore.FlagShortRunAddr == "" {
		servShortURL = "http://localhost:8080"
	} else {
		servShortURL = dataStore.configStore.FlagShortRunAddr
	}
	fmt.Fprintf(w, servShortURL+"/%s", shortURL)
}

func (dataStore *ServerDataStore) postJsonHandler(w http.ResponseWriter, r *http.Request) {
	var req models.Request
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	if req.Url == "" {
		logger.Log.Debug("after decoding JSON we don't have any URL")
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	shortURL := generateShortURL(req.Url)

	dataStore.mu.Lock()
	dataStore.urlMap[shortURL] = req.Url
	dataStore.mu.Unlock()

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

	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		logger.Log.Debug("error encoding response", zap.Error(err))
		return
	}
}

func (dataStore *ServerDataStore) getHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")

	dataStore.mu.RLock()
	originalURL, ok := dataStore.urlMap[id]
	dataStore.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func generateShortURL(url string) string {
	hash := sha256.Sum256([]byte(url))
	encoded := base64.URLEncoding.EncodeToString(hash[:])
	return encoded[:8]
}
