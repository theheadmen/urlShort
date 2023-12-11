package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	config "github.com/theheadmen/urlShort/cmd/serverconfig"

	"github.com/go-chi/chi"
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

	err := http.ListenAndServe(configStore.FlagRunAddr, makeChiServ(configStore))
	if err != nil {
		panic(err)
	}
}

func makeChiServ(configStore *config.ConfigStore) chi.Router {
	dataStore := NewServerDataStore(configStore)
	router := chi.NewRouter()
	router.Get("/", dataStore.getHandler)
	router.Get("/{shortUrl}", dataStore.getHandler)
	router.Post("/", dataStore.postHandler)
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
