package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi"
)

type ServerDataStore struct {
	urlMap map[string]string
	mu     sync.RWMutex
}

func NewServerDataStore() *ServerDataStore {
	return &ServerDataStore{
		urlMap: make(map[string]string),
		mu:     sync.RWMutex{},
	}
}

func main() {
	parseFlags()
	err := http.ListenAndServe(flagRunAddr, makeChiServ())
	if err != nil {
		panic(err)
	}
}

func makeChiServ() chi.Router {
	dataStore := NewServerDataStore()
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
	if flagShortRunAddr == "" {
		servShortURL = "http://localhost:8080"
	} else {
		servShortURL = flagShortRunAddr
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
