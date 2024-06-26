// Package serverapi хранит структуры и хендлеры для работы сервера
package serverapi

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/golang-jwt/jwt/v4"
	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	config "github.com/theheadmen/urlShort/internal/serverconfig"
	"github.com/theheadmen/urlShort/internal/storage"
	"go.uber.org/zap"

	jsoniter "github.com/json-iterator/go"
)

const (
	jwtSecretKey = "my-jwt-secret-key"
	jwtCookieKey = "token"
)

// UserClaims кастомная JWT структура
type UserClaims struct {
	UserID string `json:"userID"`
	jwt.RegisteredClaims
}

// ServerDataStore структура храняющая конфигурацию и выбранный тип хранилища для работы сервера
type ServerDataStore struct {
	configStore config.ConfigStore
	storager    storage.Storage
	json        jsoniter.API
}

// NewServerDataStore создает новый экземпляр ServerDataStore с заданными конфигурацией и хранилищем.
func NewServerDataStore(configStore *config.ConfigStore, storager storage.Storage) *ServerDataStore {
	return &ServerDataStore{
		configStore: *configStore,
		storager:    storager,
		json:        jsoniter.ConfigCompatibleWithStandardLibrary,
	}
}

// MakeChiServ создает новый экземпляр Chi-маршрутизатора и настраивает необходимые middleware.
// Он также определяет маршруты и их обработчики для сервера.
func MakeChiServ(configStore *config.ConfigStore, storager storage.Storage) chi.Router {
	dataStore := NewServerDataStore(configStore, storager)
	router := chi.NewRouter()

	// midlleware для gzip
	router.Use(middleware.Compress(5, "text/html", "application/json"))
	// middleware для куки
	router.Use(dataStore.authMiddleware)
	// middleware для логов
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

	router.Get("/", dataStore.GetHandler)
	router.Get("/{shortUrl}", dataStore.GetHandler)
	router.Post("/", dataStore.PostHandler)
	router.Post("/api/shorten", dataStore.postJSONHandler)
	router.Get("/ping", dataStore.pingHandler)
	router.Post("/api/shorten/batch", dataStore.postBatchJSONHandler)
	router.Get("/api/user/urls", dataStore.getByUserIDHandler)
	router.Delete("/api/user/urls", dataStore.deleteByUserIDHandler)
	return router
}

// PostHandler обрабатывает POST-запросы для сокращения URL.
// Он читает тело запроса, декодирует его (если необходимо), генерирует сокращенный URL,
// сохраняет его в хранилище и возвращает ответ с кодом статуса и сокращенным URL.
func (dataStore *ServerDataStore) PostHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Log.Error("cannot read request body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	url := string(body)
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(strings.NewReader(string(body)))
		if err != nil {
			logger.Log.Error("cannot decompress request body", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		decompressed, err := io.ReadAll(gz)
		if err != nil {
			logger.Log.Error("cannot read decompressed request body", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		url = string(decompressed)
	}

	token, userID, err := getTokenAndUserID(r)
	if err != nil || !token.Valid {
		logger.Log.Error("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shortURL := GenerateShortURL(url)

	isAlreadyStored, err := dataStore.storager.StoreURL(r.Context(), shortURL, url, userID)
	if err != nil {
		logger.Log.Error("cannot store url", zap.String("url", url), zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	headerStatus := http.StatusCreated
	if isAlreadyStored {
		headerStatus = http.StatusConflict
	}
	w.WriteHeader(headerStatus)
	servShortURL := dataStore.configStore.FlagShortRunAddr

	logger.Log.Info("After POST request", zap.String("body", url), zap.String("result", servShortURL+"/"+shortURL), zap.Int("userID", userID), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	fmt.Fprintf(w, servShortURL+"/%s", shortURL)
}

// postJSONHandler обрабатывает POST-запросы в формате JSON для сокращения URL.
// Он декодирует тело запроса в формате JSON, генерирует сокращенный URL,
// сохраняет его в хранилище и возвращает ответ с кодом статуса и сокращенным URL.
func (dataStore *ServerDataStore) postJSONHandler(w http.ResponseWriter, r *http.Request) {
	var req models.Request
	dec := dataStore.json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Error("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	if req.URL == "" {
		logger.Log.Debug("after decoding JSON we don't have any URL")
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	token, userID, err := getTokenAndUserID(r)
	if err != nil || !token.Valid {
		logger.Log.Error("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shortURL := GenerateShortURL(req.URL)

	isAlreadyStored, err := dataStore.storager.StoreURL(r.Context(), shortURL, req.URL, userID)
	if err != nil {
		logger.Log.Error("cannot store url", zap.String("url", req.URL), zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	headerStatus := http.StatusCreated
	if isAlreadyStored {
		headerStatus = http.StatusConflict
	}
	w.WriteHeader(headerStatus)
	servShortURL := dataStore.configStore.FlagShortRunAddr

	// заполняем модель ответа
	resp := models.Response{
		Result: servShortURL + "/" + shortURL,
	}

	logger.Log.Info("After POST JSON batch request", zap.String("body", req.URL), zap.String("result", servShortURL+"/"+shortURL), zap.Int("userID", userID), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	if err := dataStore.json.NewEncoder(w).Encode(resp); err != nil {
		logger.Log.Error("error encoding response", zap.Error(err))
		return
	}
}

// postBatchJSONHandler обрабатывает POST-запросы в формате JSON для сокращения нескольких URL.
// Он декодирует тело запроса в формате JSON, генерирует сокращенные URL,
// сохраняет их в хранилище и возвращает ответ с кодом статуса и сокращенными URL.
func (dataStore *ServerDataStore) postBatchJSONHandler(w http.ResponseWriter, r *http.Request) {
	var req []models.BatchRequest
	dec := dataStore.json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Error("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	token, userID, err := getTokenAndUserID(r)
	if err != nil || !token.Valid {
		logger.Log.Error("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	servShortURL := dataStore.configStore.FlagShortRunAddr

	var resp []models.BatchResponse
	var savedURLs []models.SavedURL
	for _, request := range req {
		if request.OriginalURL == "" {
			logger.Log.Debug("after decoding JSON we don't have any URL")
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		shortURL := GenerateShortURL(request.OriginalURL)
		savedURLs = append(savedURLs, models.SavedURL{
			UUID:        0, /*не имеет смысла, вставится автоматически потом*/
			OriginalURL: request.OriginalURL,
			ShortURL:    shortURL,
			Deleted:     false,
		})
		resp = append(resp, models.BatchResponse{
			CorrelationID: request.CorrelationID,
			ShortURL:      servShortURL + "/" + shortURL,
		})
		logger.Log.Info("Readed from batch request", zap.String("body", request.OriginalURL), zap.String("result", servShortURL+"/"+shortURL), zap.Int("userID", userID))
	}

	err = dataStore.storager.StoreURLBatch(r.Context(), savedURLs, userID)
	if err != nil {
		logger.Log.Error("cannot store urls", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	logger.Log.Info("After POST JSON request", zap.Int("count", len(resp)), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	if err := dataStore.json.NewEncoder(w).Encode(resp); err != nil {
		logger.Log.Error("error encoding response", zap.Error(err))
		return
	}
}

// getByUserIDHandler обрабатывает GET-запросы для получения всех сохраненных URL пользователя.
// Он извлекает идентификатор пользователя из токена, получает сохраненные URL из хранилища,
// и возвращает их в формате JSON.
func (dataStore *ServerDataStore) getByUserIDHandler(w http.ResponseWriter, r *http.Request) {
	token, userID, err := getTokenAndUserID(r)
	if err != nil || !token.Valid {
		logger.Log.Error("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	servShortURL := dataStore.configStore.FlagShortRunAddr

	var resp []models.BatchByUserIDResponse
	savedURLs, err := dataStore.storager.ReadAllDataForUserID(r.Context(), userID)
	if err != nil {
		logger.Log.Error("cannot read data for user", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, savedURL := range savedURLs {
		resp = append(resp, models.BatchByUserIDResponse{
			ShortURL:    servShortURL + "/" + savedURL.ShortURL,
			OriginalURL: savedURL.OriginalURL,
		})
		logger.Log.Info("Readed from batch request", zap.String("body", savedURL.OriginalURL), zap.String("result", servShortURL+"/"+savedURL.ShortURL), zap.Int("userID", userID), zap.Bool("Deleted", savedURL.Deleted))
	}

	if len(resp) == 0 {
		logger.Log.Info("We find no urls for user", zap.Int("userID", userID))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	logger.Log.Info("After POST JSON request", zap.Int("count", len(resp)), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	if err := dataStore.json.NewEncoder(w).Encode(resp); err != nil {
		logger.Log.Error("error encoding response", zap.Error(err))
		return
	}
}

// GetHandler обрабатывает GET-запросы для получения полного URL по сокращенному URL.
// Он извлекает сокращенный URL из запроса, получает полный URL из хранилища,
// и перенаправляет пользователя на исходный URL или возвращает ошибку, если URL не найден.
func (dataStore *ServerDataStore) GetHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")
	originalSavedURL, ok, err := dataStore.storager.GetURLForAnyUserID(r.Context(), id)
	if err != nil {
		logger.Log.Error("cannot get data for id", zap.String("id", id), zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !ok {
		logger.Log.Info("cannot find url by id", zap.String("id", id))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if originalSavedURL.Deleted {
		logger.Log.Info("this url is deleted", zap.String("id", id))
		w.WriteHeader(http.StatusGone)
		return
	}

	logger.Log.Info("After GET request", zap.String("id", id), zap.String("originalURL", originalSavedURL.OriginalURL))

	w.Header().Set("Location", originalSavedURL.OriginalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

// pingHandler проверяет состояние сервера и возвращает ответ с кодом статуса.
func (dataStore *ServerDataStore) pingHandler(w http.ResponseWriter, r *http.Request) {
	err := dataStore.storager.PingContext(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Log.Info("Ping succesful")
	w.WriteHeader(http.StatusOK)
}

// GenerateShortURL генерирует сокращенный URL на основе исходного URL.
func GenerateShortURL(url string) string {
	hash := sha256.Sum256([]byte(url))
	encoded := base64.RawURLEncoding.EncodeToString(hash[:])
	return encoded[:8]
}

// authMiddleware проверяет наличие и валидность токена в запросе.
// Если токен недействителен или отсутствует, он устанавливает новый токен в ответе.
func (dataStore *ServerDataStore) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the JWT from the cookie
		_, err := r.Cookie(jwtCookieKey)
		// If any other error occurred, return a bad request error
		if err != nil && err != http.ErrNoCookie {
			logger.Log.Error("error with cookie", zap.Error(err))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		isBatchByUserID := r.Method == http.MethodGet && r.RequestURI == "/api/user/urls"
		// If the cookie is not found, make a cookie
		if err == http.ErrNoCookie {
			if isBatchByUserID {
				logger.Log.Error("No cookie and isBatchByUserID", zap.Error(err))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			lastUserID, err := dataStore.storager.GetLastUserID(r.Context())
			if err != nil {
				logger.Log.Error("can't get userID for cookie", zap.Error(err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			lastUserIDStr := strconv.Itoa(lastUserID)
			setUserIDCookie(w, r, lastUserIDStr)
			dataStore.storager.SaveUserID(lastUserID)
			logger.Log.Info("Cookie is created! New user id", zap.Int("userID", lastUserID))

			next.ServeHTTP(w, r)
		} else {
			// Parse and validate the JWT
			token, userID, err := getTokenAndUserID(r)

			if err != nil || !token.Valid || !dataStore.storager.IsItCorrectUserID(userID) {
				logger.Log.Error("invalid cookie", zap.Error(err), zap.Int("userID", userID))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			logger.Log.Info("Cookie is finded", zap.Int("userID", userID))

			// If the JWT is valid, proceed to the next handler
			next.ServeHTTP(w, r)
		}
	})
}

// getTokenAndUserID извлекает токен из запроса и извлекает идентификатор пользователя из токена.
func getTokenAndUserID(r *http.Request) (*jwt.Token, int, error) {
	claims := &UserClaims{}
	cookie, err := r.Cookie(jwtCookieKey)
	// If any other error occurred, return a bad request error
	if err != nil {
		return nil, 0, err
	}

	// Parse and validate the JWT
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecretKey), nil
	})

	if err != nil {
		return nil, 0, err
	}

	if !token.Valid {
		return token, 0, fmt.Errorf("token is invalid")
	}

	userID, err := strconv.Atoi(claims.UserID)
	if err != nil {
		return token, 0, err
	}

	return token, userID, nil
}

func setUserIDCookie(w http.ResponseWriter, r *http.Request, userID string) {
	// Create a new token object, specifying signing method and the claims
	claims := UserClaims{
		userID,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			Issuer:    "myServer",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and get the complete encoded token as a string using the secret
	signedToken, err := token.SignedString([]byte(jwtSecretKey))
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	newCookie := &http.Cookie{
		Name:    jwtCookieKey,
		Value:   signedToken,
		Expires: time.Now().Add(24 * time.Hour),
	}

	r.AddCookie(newCookie)

	// Set the JWT as a cookie
	http.SetCookie(w, newCookie)
}

// GetTestCookie создает тестовый http.Cookie для использования в тестах.
func GetTestCookie() *http.Cookie {
	userID := "1"
	claims := UserClaims{
		userID,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			Issuer:    "myServer",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and get the complete encoded token as a string using the secret
	signedToken, _ := token.SignedString([]byte(jwtSecretKey))
	return &http.Cookie{
		Name:    jwtCookieKey,
		Value:   signedToken,
		Expires: time.Now().Add(24 * time.Hour),
	}
}

// deleteByUserIDHandler обрабатывает DELETE-запросы для удаления всех сохраненных URL пользователя.
// Он извлекает идентификатор пользователя из токена, удаляет сохраненные URL из хранилища,
// и возвращает ответ с кодом статуса.
func (dataStore *ServerDataStore) deleteByUserIDHandler(w http.ResponseWriter, r *http.Request) {
	token, userID, err := getTokenAndUserID(r)
	if err != nil || !token.Valid {
		logger.Log.Error("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var slice []string

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Log.Error("Error reading request body", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	err = dataStore.json.Unmarshal(body, &slice)
	if err != nil {
		logger.Log.Error("cannot decode request JSON body", zap.Error(err), zap.String("body", string(body)))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	// Print the URLs to the console
	for _, URL := range slice {
		logger.Log.Info("Try to delete", zap.String("ShortURL", URL), zap.Int("userID", userID))
	}

	// Start a new goroutine to perform the deletion
	go func() {
		// чтобы не зависеть от контекста запроса
		ctx := context.Background()
		err := dataStore.storager.DeleteByUserID(ctx, slice, userID)
		if err != nil {
			logger.Log.Info("Can't delete by user id", zap.String("error", err.Error()))
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}
