// Package file предоставляет реализацию хранилища данных, которая использует файловую систему для хранения данных.
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

// FileStorage реализует интерфейс Storage для хранения данных в файле.
type FileStorage struct {
	filePath    string
	isWithFile  bool
	URLMap      map[storage.URLMapKey]models.SavedURL
	mu          sync.RWMutex
	lastUserID  int
	usedUserIDs []int
}

// NewFileStorage создает новый экземпляр FileStorage и читает данные из файла.
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
		logger.Log.Error("Failed to read data", zap.Error(err))
	}
	return storager
}

// NewFileStoragerWithoutReadingData создает новый экземпляр FileStorage без чтения данных из файла.
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

// ReadAllData читает все данные из файла и заполняет их в FileStorage.
func (storager *FileStorage) ReadAllData(ctx context.Context) error {
	// Read from file
	file, err := os.Open(storager.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("File does not exist. Leaving SavedURLs empty.")
		} else {
			logger.Log.Error("Failed to open file", zap.Error(err))
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
			logger.Log.Error("Failed unmarshal data", zap.Error(err))
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
		logger.Log.Error("Failed to read file", zap.Error(err))
	}

	return err
}

// ReadAllDataForUserID читает все данные для определенного пользователя из файла.
func (storager *FileStorage) ReadAllDataForUserID(ctx context.Context, userID int) ([]models.SavedURL, error) {
	filteredData := []models.SavedURL{}
	// Read from file
	file, err := os.Open(storager.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("File does not exist. Leaving SavedURLs empty.")
		} else {
			logger.Log.Error("Failed to open file", zap.Error(err))
		}
		return []models.SavedURL{}, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result models.SavedURL
		err := json.Unmarshal([]byte(scanner.Text()), &result)
		if err != nil {
			logger.Log.Error("Failed unmarshal data", zap.Error(err))
		}
		// запоминаем только то, что связано с нужным пользователем
		if result.UserID == userID {
			filteredData = append(filteredData, result)
			logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL), zap.Int("UserID", result.UserID), zap.Bool("Deleted", result.Deleted))
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Log.Error("Failed to read file", zap.Error(err))
	}

	return filteredData, err
}

// StoreURL сохраняет URL в FileStorage и файл.
func (storager *FileStorage) StoreURL(ctx context.Context, shortURL string, originalURL string, userID int) (bool, error) {
	_, ok := storager.GetURL(shortURL, userID)

	if ok {
		logger.Log.Info("We already have data for this url", zap.String("OriginalURL", originalURL), zap.String("ShortURL", shortURL), zap.Bool("Deleted", false))
		return true, nil
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
	return false, nil
}

// StoreURLBatch сохраняет несколько URL в FileStorage и файл.
func (storager *FileStorage) StoreURLBatch(ctx context.Context, forStore []models.SavedURL, userID int) error {
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

	return nil
}

// Save сохраняет URL в файл.
func (storager *FileStorage) Save(savedURL models.SavedURL) error {
	savedURLJSON, err := json.Marshal(savedURL)
	if err != nil {
		logger.Log.Error("Failed to marshal new data", zap.Error(err))
		return err
	}
	file, err := os.OpenFile(storager.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Log.Error("Failed to open file for writing", zap.Error(err))
		return err
	}
	defer file.Close()

	savedURLJSON = append(savedURLJSON, '\n')
	if _, err := file.Write(savedURLJSON); err != nil {
		logger.Log.Error("Failed to write to file", zap.Error(err))
		return err
	}
	logger.Log.Info("Write new data to file", zap.Int("UUID", savedURL.UUID), zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("UserID", savedURL.UserID))
	return nil
}

// GetURL возвращает URL из FileStorage.
func (storager *FileStorage) GetURL(shortURL string, userID int) (string, bool) {
	storager.mu.RLock()
	originalSavedURL, ok := storager.URLMap[storage.URLMapKey{ShortURL: shortURL, UserID: userID}]
	storager.mu.RUnlock()

	return originalSavedURL.OriginalURL, ok
}

// GetURLForAnyUserID возвращает URL, независимо от пользователя.
func (storager *FileStorage) GetURLForAnyUserID(ctx context.Context, shortURL string) (models.SavedURL, bool, error) {
	storager.mu.RLock()
	originalSavedURL, ok := storager.findEntityByShortURL(shortURL)
	storager.mu.RUnlock()

	return originalSavedURL, ok, nil
}

// findEntityByShortURL ищет первый полный URL для заданного короткого URL
func (storager *FileStorage) findEntityByShortURL(shortURL string) (models.SavedURL, bool) {
	for key, value := range storager.URLMap {
		if key.ShortURL == shortURL {
			return value, true
		}
	}
	return models.SavedURL{}, false
}

// IsItCorrectUserID проверяет, является ли идентификатор пользователя корректным.
func (storager *FileStorage) IsItCorrectUserID(userID int) bool {
	storager.mu.RLock()
	ok := storager.findUserID(userID)
	storager.mu.RUnlock()

	return ok
}

// findUserID ищет пользователя по заданному ID
func (storager *FileStorage) findUserID(userID int) bool {
	for _, usedUserID := range storager.usedUserIDs {
		if usedUserID == userID {
			return true
		}
	}
	return false
}

// GetLastUserID возвращает последний использованный идентификатор пользователя.
func (storager *FileStorage) GetLastUserID(ctx context.Context) (int, error) {
	storager.lastUserID = storager.lastUserID + 1
	return storager.lastUserID, nil
}

// SaveUserID сохраняет идентификатор пользователя.
func (storager *FileStorage) SaveUserID(userID int) {
	storager.mu.Lock()
	storager.usedUserIDs = append(storager.usedUserIDs, userID)
	storager.mu.Unlock()
}

// DeleteByUserID удаляет URL, принадлежащие определенному пользователю.
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

// PingContext проверяет соединение с хранилищем.
func (storager *FileStorage) PingContext(ctx context.Context) error {
	logger.Log.Info("db is not alive, we don't need to ping")
	return fmt.Errorf("db is not alive, we don't need to ping")
}
