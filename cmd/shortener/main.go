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

	"github.com/theheadmen/urlShort/cmd/dbconnector"
	"github.com/theheadmen/urlShort/cmd/logger"
	"github.com/theheadmen/urlShort/cmd/models"
	config "github.com/theheadmen/urlShort/cmd/serverconfig"
	"github.com/theheadmen/urlShort/cmd/storager"
	"go.uber.org/zap"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"

	_ "github.com/lib/pq"
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
	dbConnector := dbconnector.NewDBConnector(configStore.FlagDB)
	storager := storager.NewStorager(configStore.FlagFile, true /*isWithFile*/, make(map[string]string), dbConnector)
	storager.ReadAllData()

	err := http.ListenAndServe(configStore.FlagRunAddr, makeChiServ(configStore, storager))
	if err != nil {
		logger.Log.Fatal("Server is down", zap.String("address", err.Error()))
		panic(err)
	}
}

func makeChiServ(configStore *config.ConfigStore, storager *storager.Storager) chi.Router {
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
	router.Get("/ping", dataStore.pingHandler)
	router.Post("/api/shorten/batch", dataStore.postBatchJSONHandler)
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

	dataStore.storager.StoreURL(shortURL, url)

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

	dataStore.storager.StoreURL(shortURL, req.URL)

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

	logger.Log.Info("After POST JSON batch request", zap.String("body", req.URL), zap.String("result", servShortURL+"/"+shortURL), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		logger.Log.Debug("error encoding response", zap.Error(err))
		return
	}
}

func (dataStore *ServerDataStore) postBatchJSONHandler(w http.ResponseWriter, r *http.Request) {
	var req []models.BatchRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	servShortURL := ""
	// так как в тестах мы не используем флаги, нужно обезопасить себя
	if dataStore.configStore.FlagShortRunAddr == "" {
		servShortURL = "http://localhost:8080"
	} else {
		servShortURL = dataStore.configStore.FlagShortRunAddr
	}

	var resp []models.BatchResponse
	var savedURLs []models.SavedURL
	for _, request := range req {
		if request.OriginalURL == "" {
			logger.Log.Debug("after decoding JSON we don't have any URL")
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		shortURL := generateShortURL(request.OriginalURL)
		savedURLs = append(savedURLs, models.SavedURL{
			UUID:        0, /*не имеет смысла, вставится автоматически потом*/
			OriginalURL: request.OriginalURL,
			ShortURL:    shortURL,
		})
		resp = append(resp, models.BatchResponse{
			CorrelationID: request.CorrelationID,
			ShortURL:      servShortURL + "/" + shortURL,
		})
		logger.Log.Info("Readed from batch request", zap.String("body", request.OriginalURL), zap.String("result", servShortURL+"/"+shortURL))
	}

	dataStore.storager.StoreURLBatch(savedURLs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	logger.Log.Info("After POST JSON request", zap.Int("count", len(resp)), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

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
		logger.Log.Info("cannot find url by id", zap.String("id", id))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logger.Log.Info("After GET request", zap.String("id", id), zap.String("originalURL", originalURL))

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (dataStore *ServerDataStore) pingHandler(w http.ResponseWriter, r *http.Request) {
	if !dataStore.storager.DB.IsAlive {
		logger.Log.Info("DB is not alive, we don't need to ping")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err := dataStore.storager.DB.DB.Ping()
	if err != nil {
		logger.Log.Info("Can't ping DB", zap.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Log.Info("Ping succesful")
	w.WriteHeader(http.StatusOK)
}

func generateShortURL(url string) string {
	hash := sha256.Sum256([]byte(url))
	encoded := base64.RawURLEncoding.EncodeToString(hash[:])
	return encoded[:8]
}
