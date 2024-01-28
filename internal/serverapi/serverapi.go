package serverapi

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	"github.com/theheadmen/urlShort/internal/storager"
	"go.uber.org/zap"
)

const (
	jwtSecretKey = "my-jwt-secret-key"
	jwtCookieKey = "token"
)

// UserClaims is a custom JWT claims structure
type UserClaims struct {
	UserID string `json:"userID"`
	jwt.RegisteredClaims
}

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

func MakeChiServ(configStore *config.ConfigStore, storager *storager.Storager) chi.Router {
	dataStore := NewServerDataStore(configStore, storager)
	router := chi.NewRouter()

	// Add gzip middleware
	router.Use(middleware.Compress(5, "text/html", "application/json"))
	// cookie middleware
	router.Use(dataStore.authMiddleware)
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

	router.Get("/", dataStore.GetHandler)
	router.Get("/{shortUrl}", dataStore.GetHandler)
	router.Post("/", dataStore.PostHandler)
	router.Post("/api/shorten", dataStore.postJSONHandler)
	router.Get("/ping", dataStore.pingHandler)
	router.Post("/api/shorten/batch", dataStore.postBatchJSONHandler)
	router.Get("/api/user/urls", dataStore.getByUserIDHandler)
	return router
}

func (dataStore *ServerDataStore) PostHandler(w http.ResponseWriter, r *http.Request) {
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

	cookie, err := r.Cookie(jwtCookieKey)
	// If any other error occurred, return a bad request error
	if err != nil {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		for _, cookie := range r.Cookies() {

			logger.Log.Info("cookie that we have", zap.String("name", cookie.Name), zap.String("Value", cookie.Value))
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	token, userID, err := getTokenAndUserId(cookie)
	if err != nil || !token.Valid {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shortURL := GenerateShortURL(url)

	isAlreadyStored := dataStore.storager.StoreURL(r.Context(), shortURL, url, userID)

	w.Header().Set("Content-Type", "text/html")
	headerStatus := http.StatusCreated
	if isAlreadyStored {
		headerStatus = http.StatusConflict
	}
	w.WriteHeader(headerStatus)
	servShortURL := ""
	// так как в тестах мы не используем флаги, нужно обезопасить себя
	if dataStore.configStore.FlagShortRunAddr == "" {
		servShortURL = "http://localhost:8080"
	} else {
		servShortURL = dataStore.configStore.FlagShortRunAddr
	}

	logger.Log.Info("After POST request", zap.String("body", url), zap.String("result", servShortURL+"/"+shortURL), zap.Int("userID", userID), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

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

	cookie, err := r.Cookie(jwtCookieKey)
	// If any other error occurred, return a bad request error
	if err != nil {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	token, userID, err := getTokenAndUserId(cookie)
	if err != nil || !token.Valid {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shortURL := GenerateShortURL(req.URL)

	isAlreadyStored := dataStore.storager.StoreURL(r.Context(), shortURL, req.URL, userID)

	w.Header().Set("Content-Type", "application/json")
	headerStatus := http.StatusCreated
	if isAlreadyStored {
		headerStatus = http.StatusConflict
	}
	w.WriteHeader(headerStatus)
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

	logger.Log.Info("After POST JSON batch request", zap.String("body", req.URL), zap.String("result", servShortURL+"/"+shortURL), zap.Int("userID", userID), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

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

	cookie, err := r.Cookie(jwtCookieKey)
	// If any other error occurred, return a bad request error
	if err != nil {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	token, userID, err := getTokenAndUserId(cookie)
	if err != nil || !token.Valid {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
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

		shortURL := GenerateShortURL(request.OriginalURL)
		savedURLs = append(savedURLs, models.SavedURL{
			UUID:        0, /*не имеет смысла, вставится автоматически потом*/
			OriginalURL: request.OriginalURL,
			ShortURL:    shortURL,
		})
		resp = append(resp, models.BatchResponse{
			CorrelationID: request.CorrelationID,
			ShortURL:      servShortURL + "/" + shortURL,
		})
		logger.Log.Info("Readed from batch request", zap.String("body", request.OriginalURL), zap.String("result", servShortURL+"/"+shortURL), zap.Int("userID", userID))
	}

	dataStore.storager.StoreURLBatch(r.Context(), savedURLs, userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	logger.Log.Info("After POST JSON request", zap.Int("count", len(resp)), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		logger.Log.Debug("error encoding response", zap.Error(err))
		return
	}
}

func (dataStore *ServerDataStore) getByUserIDHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(jwtCookieKey)
	// If any other error occurred, return a bad request error
	if err != nil {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	token, userID, err := getTokenAndUserId(cookie)
	if err != nil || !token.Valid {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	servShortURL := ""
	// так как в тестах мы не используем флаги, нужно обезопасить себя
	if dataStore.configStore.FlagShortRunAddr == "" {
		servShortURL = "http://localhost:8080"
	} else {
		servShortURL = dataStore.configStore.FlagShortRunAddr
	}

	var resp []models.BatchByUserIDResponse
	savedURLs, err := dataStore.storager.ReadAllDataForUserID(r.Context(), userID)
	if err != nil {
		logger.Log.Info("cannot read data for user", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, savedURL := range savedURLs {
		resp = append(resp, models.BatchByUserIDResponse{
			ShortURL:    servShortURL + "/" + savedURL.ShortURL,
			OriginalURL: servShortURL + "/" + savedURL.OriginalURL,
		})
		logger.Log.Info("Readed from batch request", zap.String("body", savedURL.OriginalURL), zap.String("result", servShortURL+"/"+savedURL.ShortURL), zap.Int("userID", userID))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	logger.Log.Info("After POST JSON request", zap.Int("count", len(resp)), zap.String("content-encoding", r.Header.Get("Content-Encoding")))

	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		logger.Log.Debug("error encoding response", zap.Error(err))
		return
	}
}

func (dataStore *ServerDataStore) GetHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")

	cookie, err := r.Cookie(jwtCookieKey)
	// If any other error occurred, return a bad request error
	if err != nil {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	token, userID, err := getTokenAndUserId(cookie)
	if err != nil || !token.Valid {
		logger.Log.Info("cannot find cookie", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	originalURL, ok := dataStore.storager.GetURL(id, userID)

	if !ok {
		logger.Log.Info("cannot find url by id", zap.String("id", id), zap.Int("userID", userID))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logger.Log.Info("After GET request", zap.String("id", id), zap.String("originalURL", originalURL), zap.Int("userID", userID))

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (dataStore *ServerDataStore) pingHandler(w http.ResponseWriter, r *http.Request) {
	if dataStore.storager.DB == nil {
		logger.Log.Info("DB is not alive, we don't need to ping")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err := dataStore.storager.DB.DB.PingContext(r.Context())
	if err != nil {
		logger.Log.Info("Can't ping DB", zap.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Log.Info("Ping succesful")
	w.WriteHeader(http.StatusOK)
}

func GenerateShortURL(url string) string {
	hash := sha256.Sum256([]byte(url))
	encoded := base64.RawURLEncoding.EncodeToString(hash[:])
	return encoded[:8]
}

func (dataStore *ServerDataStore) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Log.Info("CHECK COOKIE")
		// Get the JWT from the cookie
		cookie, err := r.Cookie(jwtCookieKey)
		// If any other error occurred, return a bad request error
		if err != nil && err != http.ErrNoCookie {
			logger.Log.Info("error with cookie", zap.Error(err))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// If the cookie is not found, return an unauthorized error
		if err == http.ErrNoCookie {
			logger.Log.Info("Time to create cookie!")
			lastUserID, err := dataStore.storager.GetLastUserID(r.Context())
			if err != nil {
				logger.Log.Info("can't get userID for cookie", zap.Error(err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			lastUserIDStr := strconv.Itoa(lastUserID)
			setUserIDCookie(w, r, lastUserIDStr)
			logger.Log.Info("Cookie is created!")

			next.ServeHTTP(w, r)
		} else {
			// Parse and validate the JWT
			token, _, err := getTokenAndUserId(cookie)

			if err != nil || !token.Valid {
				logger.Log.Info("invalid cookie", zap.Error(err))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// If the JWT is valid, proceed to the next handler
			next.ServeHTTP(w, r)
		}
	})
}

func getTokenAndUserId(cookie *http.Cookie) (*jwt.Token, int, error) {
	claims := &UserClaims{}

	// Parse and validate the JWT
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecretKey), nil
	})

	if err != nil || !token.Valid {
		return token, 0, err
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
