package file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/storage"
	"go.uber.org/zap"
)

type FileStorage struct {
	filePath    string
	isWithFile  bool
	URLMap      map[storage.URLMapKey]models.SavedURL
	mu          sync.RWMutex
	lastUserID  int
	usedUserIDs []int
}

func NewFileStorage(filePath string, isWithFile bool, URLMap map[storage.URLMapKey]models.SavedURL, ctx context.Context) *FileStorage {
	var empty []int

	storager := &FileStorage{
		filePath:    filePath,
		isWithFile:  isWithFile,
		URLMap:      URLMap,
		mu:          sync.RWMutex{},
		lastUserID:  0,
		usedUserIDs: empty,
	}
	err := storager.ReadAllData(ctx)
	if err != nil {
		logger.Log.Info("Failed to read data", zap.Error(err))
	}
	return storager
}

func NewFileStoragerWithoutReadingData(filePath string, isWithFile bool, URLMap map[storage.URLMapKey]models.SavedURL) *FileStorage {
	return &FileStorage{
		filePath:    filePath,
		isWithFile:  isWithFile,
		URLMap:      URLMap,
		mu:          sync.RWMutex{},
		lastUserID:  1,
		usedUserIDs: []int{1},
	}
}

func NewFileStorageWithoutReadingData(filePath string, isWithFile bool, URLMap map[storage.URLMapKey]models.SavedURL) *FileStorage {
	return &FileStorage{
		filePath:    filePath,
		isWithFile:  isWithFile,
		URLMap:      URLMap,
		mu:          sync.RWMutex{},
		lastUserID:  1,
		usedUserIDs: []int{1},
	}
}

func (storager *FileStorage) ReadAllData(ctx context.Context) error {
	// Read from file
	file, err := os.Open(storager.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("File does not exist. Leaving SavedURLs empty.")
		} else {
			logger.Log.Info("Failed to open file", zap.Error(err))
		}
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	curMax := storager.lastUserID

	for scanner.Scan() {
		var result models.SavedURL
		err := json.Unmarshal([]byte(scanner.Text()), &result)
		if err != nil {
			logger.Log.Debug("Failed unmarshal data", zap.Error(err))
		}
		storager.URLMap[storage.URLMapKey{ShortURL: result.ShortURL, UserID: result.UserID}] = result
		storager.usedUserIDs = append(storager.usedUserIDs, result.UserID)
		// запоминаем максимальный userId, чтобы выдавать следующий за ним
		if result.UserID > curMax {
			curMax = result.UserID
		}
		logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL), zap.Int("UserID", result.UserID), zap.Bool("Deleted", result.Deleted))
	}
	storager.lastUserID = curMax

	if err := scanner.Err(); err != nil {
		logger.Log.Info("Failed to read file", zap.Error(err))
	}

	return err
}

func (storager *FileStorage) ReadAllDataForUserID(ctx context.Context, userID int) ([]models.SavedURL, error) {
	filteredData := []models.SavedURL{}
	// Read from file
	file, err := os.Open(storager.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("File does not exist. Leaving SavedURLs empty.")
		} else {
			logger.Log.Info("Failed to open file", zap.Error(err))
		}
		return []models.SavedURL{}, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result models.SavedURL
		err := json.Unmarshal([]byte(scanner.Text()), &result)
		if err != nil {
			logger.Log.Debug("Failed unmarshal data", zap.Error(err))
		}
		// запоминаем только то, что связано с нужным пользователем
		if result.UserID == userID {
			filteredData = append(filteredData, result)
			logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL), zap.Int("UserID", result.UserID), zap.Bool("Deleted", result.Deleted))
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Log.Info("Failed to read file", zap.Error(err))
	}

	return filteredData, err
}

// возвращает true если это значение уже было записано ранее
func (storager *FileStorage) StoreURL(ctx context.Context, shortURL string, originalURL string, userID int) bool {
	_, ok := storager.GetURL(shortURL, userID)

	if ok {
		logger.Log.Info("We already have data for this url", zap.String("OriginalURL", originalURL), zap.String("ShortURL", shortURL), zap.Bool("Deleted", false))
		return true
	}

	savedURL := models.SavedURL{
		UUID:        len(storager.URLMap),
		ShortURL:    shortURL,
		OriginalURL: originalURL,
		UserID:      userID,
		Deleted:     false,
	}

	storager.mu.Lock()
	storager.URLMap[storage.URLMapKey{ShortURL: shortURL, UserID: userID}] = savedURL
	storager.mu.Unlock()

	storager.Save(savedURL)
	return false
}

func (storager *FileStorage) StoreURLBatch(ctx context.Context, forStore []models.SavedURL, userID int) {
	var filteredStore []models.SavedURL
	for _, savedURL := range forStore {
		_, ok := storager.GetURL(savedURL.ShortURL, userID)

		if ok {
			logger.Log.Info("We already have data for this url", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("UserID", userID), zap.Bool("Deleted", savedURL.Deleted))
		} else {
			storager.mu.Lock()
			storager.URLMap[storage.URLMapKey{ShortURL: savedURL.ShortURL, UserID: userID}] = savedURL
			storager.mu.Unlock()
			filteredStore = append(filteredStore, savedURL)
		}
	}
	// если у нас уже все и так было вставлено, нам не нужно ничего сохранять
	if len(filteredStore) != 0 {
		if storager.isWithFile {
			for _, savedURL := range filteredStore {
				storager.Save(savedURL)
			}
		}
	}
}

func (storager *FileStorage) Save(savedURL models.SavedURL) error {
	savedURLJSON, err := json.Marshal(savedURL)
	if err != nil {
		logger.Log.Info("Failed to marshal new data", zap.Error(err))
		return err
	}
	file, err := os.OpenFile(storager.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Log.Info("Failed to open file for writing", zap.Error(err))
		return err
	}
	defer file.Close()

	savedURLJSON = append(savedURLJSON, '\n')
	if _, err := file.Write(savedURLJSON); err != nil {
		logger.Log.Info("Failed to write to file", zap.Error(err))
		return err
	}
	logger.Log.Info("Write new data to file", zap.Int("UUID", savedURL.UUID), zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("UserID", savedURL.UserID))
	return nil
}

func (storager *FileStorage) GetURL(shortURL string, userID int) (string, bool) {
	storager.mu.RLock()
	originalSavedURL, ok := storager.URLMap[storage.URLMapKey{ShortURL: shortURL, UserID: userID}]
	storager.mu.RUnlock()

	return originalSavedURL.OriginalURL, ok
}

func (storager *FileStorage) GetURLForAnyUserID(shortURL string) (models.SavedURL, bool) {
	storager.mu.RLock()
	originalSavedURL, ok := storager.findEntityByShortURL(shortURL)
	storager.mu.RUnlock()

	return originalSavedURL, ok
}

func (storager *FileStorage) findEntityByShortURL(shortURL string) (models.SavedURL, bool) {
	for key, value := range storager.URLMap {
		if key.ShortURL == shortURL {
			return value, true
		}
	}
	return models.SavedURL{}, false
}

func (storager *FileStorage) IsItCorrectUserID(userID int) bool {
	storager.mu.RLock()
	ok := storager.findUserID(userID)
	storager.mu.RUnlock()

	return ok
}

func (storager *FileStorage) findUserID(userID int) bool {
	for _, usedUserID := range storager.usedUserIDs {
		if usedUserID == userID {
			return true
		}
	}
	return false
}

func (storager *FileStorage) GetLastUserID(ctx context.Context) (int, error) {
	storager.lastUserID = storager.lastUserID + 1
	return storager.lastUserID, nil
}

func (storager *FileStorage) SaveUserID(userID int) {
	storager.mu.Lock()
	storager.usedUserIDs = append(storager.usedUserIDs, userID)
	storager.mu.Unlock()
}

func (storager *FileStorage) DeleteByUserID(ctx context.Context, shortURLs []string, userID int) error {
	storager.mu.Lock()
	for _, shortURL := range shortURLs {
		originalSavedURL, ok := storager.findEntityByShortURL(shortURL)
		if ok {
			originalSavedURL.Deleted = true
			storager.URLMap[storage.URLMapKey{ShortURL: shortURL, UserID: userID}] = originalSavedURL
		}
	}
	storager.mu.Unlock()

	if storager.isWithFile {
		// а что с файлом делать? Просто дописать?
		logger.Log.Info("Update file")
		filteredStore := []models.SavedURL{}

		storager.mu.RLock()
		for _, shortURL := range shortURLs {
			originalSavedURL, ok := storager.URLMap[storage.URLMapKey{ShortURL: shortURL, UserID: userID}]
			if ok {
				filteredStore = append(filteredStore, originalSavedURL)
			}
		}
		storager.mu.RUnlock()

		for _, savedURL := range filteredStore {
			storager.Save(savedURL)
		}
		return nil
	}
	return nil
}

func (storager *FileStorage) PingContext(ctx context.Context) error {
	logger.Log.Info("DB is not alive, we don't need to ping")
	return fmt.Errorf("DB is not alive, we don't need to ping")
}
