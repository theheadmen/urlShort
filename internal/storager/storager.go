package storager

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/theheadmen/urlShort/internal/dbconnector"
	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	"go.uber.org/zap"
)

type UrlMapKey struct {
	shortURL string
	userID   int
}

type Storager struct {
	filePath   string
	isWithFile bool
	URLMap     map[UrlMapKey]string
	mu         sync.RWMutex
	DB         *dbconnector.DBConnector
	lastUserID int
}

func NewStorager(filePath string, isWithFile bool, URLMap map[UrlMapKey]string, dbConnector *dbconnector.DBConnector, ctx context.Context) *Storager {
	storager := &Storager{
		filePath:   filePath,
		isWithFile: isWithFile,
		URLMap:     URLMap,
		mu:         sync.RWMutex{},
		DB:         dbConnector,
		lastUserID: 0,
	}
	err := storager.readAllData(ctx)
	if err != nil {
		logger.Log.Info("Failed to read data", zap.Error(err))
	}
	return storager
}

func NewStoragerWithoutReadingData(filePath string, isWithFile bool, URLMap map[UrlMapKey]string, dbConnector *dbconnector.DBConnector) *Storager {
	return &Storager{
		filePath:   filePath,
		isWithFile: isWithFile,
		URLMap:     URLMap,
		mu:         sync.RWMutex{},
		DB:         dbConnector,
		lastUserID: 0,
	}
}

func (storager *Storager) readAllData(ctx context.Context) error {
	if storager.DB != nil {
		urls, err := storager.DB.SelectAllSavedURLs(ctx)
		if err != nil {
			logger.Log.Fatal("Failed to read from database", zap.Error(err))
			return err
		}

		for _, url := range urls {
			storager.URLMap[UrlMapKey{url.ShortURL, url.UserID}] = url.OriginalURL
			logger.Log.Info("Read new data from database", zap.Int("UUID", url.UUID), zap.String("OriginalURL", url.OriginalURL), zap.String("ShortURL", url.ShortURL), zap.Int("UserID", url.UserID))
		}

		return err
	}

	return storager.ReadAllDataFromFile()
}

func (storager *Storager) ReadAllDataFromFile() error {
	// Read from file
	file, err := os.Open(storager.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("File does not exist. Leaving SavedURLs empty.")
		} else {
			logger.Log.Fatal("Failed to open file", zap.Error(err))
		}
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	curMax := storager.lastUserID
	// здесь нужно искать максимальное значение userID
	for scanner.Scan() {
		var result models.SavedURL
		err := json.Unmarshal([]byte(scanner.Text()), &result)
		if err != nil {
			logger.Log.Debug("Failed unmarshal data", zap.Error(err))
		}
		storager.URLMap[UrlMapKey{result.ShortURL, result.UserID}] = result.OriginalURL
		if result.UserID > curMax {
			curMax = result.UserID
		}
		logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL), zap.Int("UserID", result.UserID))
	}
	storager.lastUserID = curMax

	if err := scanner.Err(); err != nil {
		logger.Log.Fatal("Failed to read file", zap.Error(err))
	}

	return err
}

func (storager *Storager) ReadAllDataForUserID(ctx context.Context, userID int) ([]models.SavedURL, error) {
	if storager.DB != nil {
		urls, err := storager.DB.SelectSavedURLsForUserID(ctx, userID)
		if err != nil {
			logger.Log.Fatal("Failed to read from database", zap.Error(err))
			return []models.SavedURL{}, err
		}

		return urls, err
	}

	return storager.ReadAllDataFromFileForUserID(userID)
}

func (storager *Storager) ReadAllDataFromFileForUserID(userID int) ([]models.SavedURL, error) {
	filteredData := []models.SavedURL{}
	// Read from file
	file, err := os.Open(storager.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("File does not exist. Leaving SavedURLs empty.")
		} else {
			logger.Log.Fatal("Failed to open file", zap.Error(err))
		}
		return []models.SavedURL{}, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	// здесь нужно искать максимальное значение userID
	for scanner.Scan() {
		var result models.SavedURL
		err := json.Unmarshal([]byte(scanner.Text()), &result)
		if err != nil {
			logger.Log.Debug("Failed unmarshal data", zap.Error(err))
		}
		if result.UserID == userID {
			filteredData = append(filteredData, result)
			logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL), zap.Int("UserID", result.UserID))
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Log.Fatal("Failed to read file", zap.Error(err))
	}

	return filteredData, err
}

// возвращает true если это значение уже было записано ранее
func (storager *Storager) StoreURL(ctx context.Context, shortURL string, originalURL string, userID int) bool {
	_, ok := storager.GetURL(shortURL, userID)

	if ok {
		logger.Log.Info("We already have data for this url", zap.String("OriginalURL", originalURL), zap.String("ShortURL", shortURL))
		return true
	}

	storager.mu.Lock()
	storager.URLMap[UrlMapKey{shortURL, userID}] = originalURL
	storager.mu.Unlock()

	savedURL := models.SavedURL{
		UUID:        len(storager.URLMap),
		ShortURL:    shortURL,
		OriginalURL: originalURL,
		UserID:      userID,
	}

	if storager.DB != nil {
		storager.DB.InsertSavedURLBatch(ctx, []models.SavedURL{savedURL}, userID)
	} else if storager.isWithFile {
		storager.Save(savedURL)
	}
	return false
}

func (storager *Storager) StoreURLBatch(ctx context.Context, forStore []models.SavedURL, userID int) {
	var filteredStore []models.SavedURL
	for _, savedURL := range forStore {
		_, ok := storager.GetURL(savedURL.ShortURL, userID)

		if ok {
			logger.Log.Info("We already have data for this url", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("UserID", userID))
		} else {
			storager.mu.Lock()
			storager.URLMap[UrlMapKey{savedURL.ShortURL, userID}] = savedURL.OriginalURL
			storager.mu.Unlock()
			filteredStore = append(filteredStore, savedURL)
		}
	}
	// если у нас уже все и так было вставлено, нам не нужно ничего сохранять
	if len(filteredStore) != 0 {
		if storager.DB != nil {
			storager.DB.InsertSavedURLBatch(ctx, filteredStore, userID)
		} else if storager.isWithFile {
			for _, savedURL := range filteredStore {
				storager.Save(savedURL)
			}
		}
	}
}

func (storager *Storager) Save(savedURL models.SavedURL) error {
	savedURLJSON, err := json.Marshal(savedURL)
	if err != nil {
		logger.Log.Fatal("Failed to marshal new data", zap.Error(err))
		return err
	}
	file, err := os.OpenFile(storager.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Log.Fatal("Failed to open file for writing", zap.Error(err))
		return err
	}
	defer file.Close()

	savedURLJSON = append(savedURLJSON, '\n')
	if _, err := file.Write(savedURLJSON); err != nil {
		logger.Log.Fatal("Failed to write to file", zap.Error(err))
		return err
	}
	logger.Log.Info("Write new data to file", zap.Int("UUID", savedURL.UUID), zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("UserID", savedURL.UserID))
	return nil
}

func (storager *Storager) GetURL(shortURL string, userID int) (string, bool) {
	storager.mu.RLock()
	originalURL, ok := storager.URLMap[UrlMapKey{shortURL, userID}]
	storager.mu.RUnlock()

	return originalURL, ok
}

func (storager *Storager) GetLastUserID(ctx context.Context) (int, error) {
	if storager.DB != nil {
		lastUserID, err := storager.DB.IncrementId(ctx)
		if err != nil {
			logger.Log.Fatal("Failed to read last user id from database", zap.Error(err))
			return lastUserID, err
		}

		return lastUserID, nil
	}

	storager.lastUserID = storager.lastUserID + 1
	return storager.lastUserID, nil
}
