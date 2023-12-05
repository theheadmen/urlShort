package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
)

var urlMap = make(map[string]string)

func main() {
	parseFlags()
	http.ListenAndServe(flagRunAddr, makeChiServ())
}

func makeChiServ() chi.Router {
	r := chi.NewRouter()
	r.Get("/", getHandler)
	r.Get("/{shortUrl}", getHandler)
	r.Post("/", postHandler)
	return r
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	url := string(body)
	shortURL := generateShortURL(url)
	urlMap[shortURL] = url
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	servShortUrl := ""
	// так как в тестах мы не используем флаги, нужно обезопасить себя
	if flagShortRunAddr == "" {
		servShortUrl = "http://localhost:8080"
	} else {
		servShortUrl = flagShortRunAddr
	}
	fmt.Fprintf(w, servShortUrl+"/%s", shortURL)
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")
	originalURL, ok := urlMap[id]
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
