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

type URLMapKey struct {
	shortURL string
	userID   int
}

type Storager struct {
	filePath    string
	isWithFile  bool
	URLMap      map[URLMapKey]models.SavedURL
	mu          sync.RWMutex
	DB          *dbconnector.DBConnector
	lastUserID  int
	usedUserIDs []int
}

func NewStorager(filePath string, isWithFile bool, URLMap map[URLMapKey]models.SavedURL, dbConnector *dbconnector.DBConnector, ctx context.Context) *Storager {
	var empty []int

	storager := &Storager{
		filePath:    filePath,
		isWithFile:  isWithFile,
		URLMap:      URLMap,
		mu:          sync.RWMutex{},
		DB:          dbConnector,
		lastUserID:  0,
		usedUserIDs: empty,
	}
	err := storager.readAllData(ctx)
	if err != nil {
		logger.Log.Info("Failed to read data", zap.Error(err))
	}
	return storager
}

func NewStoragerWithoutReadingData(filePath string, isWithFile bool, URLMap map[URLMapKey]models.SavedURL, dbConnector *dbconnector.DBConnector) *Storager {
	return &Storager{
		filePath:    filePath,
		isWithFile:  isWithFile,
		URLMap:      URLMap,
		mu:          sync.RWMutex{},
		DB:          dbConnector,
		lastUserID:  1,
		usedUserIDs: []int{1},
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
			storager.URLMap[URLMapKey{url.ShortURL, url.UserID}] = url
			storager.usedUserIDs = append(storager.usedUserIDs, url.UserID)
			logger.Log.Info("Read new data from database", zap.Int("UUID", url.UUID), zap.String("OriginalURL", url.OriginalURL), zap.String("ShortURL", url.ShortURL), zap.Int("UserID", url.UserID), zap.Bool("Deleted", url.Deleted))
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

	for scanner.Scan() {
		var result models.SavedURL
		err := json.Unmarshal([]byte(scanner.Text()), &result)
		if err != nil {
			logger.Log.Debug("Failed unmarshal data", zap.Error(err))
		}
		storager.URLMap[URLMapKey{result.ShortURL, result.UserID}] = result
		storager.usedUserIDs = append(storager.usedUserIDs, result.UserID)
		// запоминаем максимальный userId, чтобы выдавать следующий за ним
		if result.UserID > curMax {
			curMax = result.UserID
		}
		logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL), zap.Int("UserID", result.UserID), zap.Bool("Deleted", result.Deleted))
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
		logger.Log.Fatal("Failed to read file", zap.Error(err))
	}

	return filteredData, err
}

// возвращает true если это значение уже было записано ранее
func (storager *Storager) StoreURL(ctx context.Context, shortURL string, originalURL string, userID int) bool {
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
	storager.URLMap[URLMapKey{shortURL, userID}] = savedURL
	storager.mu.Unlock()

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
			logger.Log.Info("We already have data for this url", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("UserID", userID), zap.Bool("Deleted", savedURL.Deleted))
		} else {
			storager.mu.Lock()
			storager.URLMap[URLMapKey{savedURL.ShortURL, userID}] = savedURL
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
	originalSavedURL, ok := storager.URLMap[URLMapKey{shortURL, userID}]
	storager.mu.RUnlock()

	return originalSavedURL.OriginalURL, ok
}

func (storager *Storager) GetURLForAnyUserID(shortURL string) (models.SavedURL, bool) {
	storager.mu.RLock()
	originalSavedURL, ok := storager.findEntityByShortURL(shortURL)
	storager.mu.RUnlock()

	return originalSavedURL, ok
}

func (storager *Storager) findEntityByShortURL(shortURL string) (models.SavedURL, bool) {
	for key, value := range storager.URLMap {
		if key.shortURL == shortURL {
			return value, true
		}
	}
	return models.SavedURL{}, false
}

func (storager *Storager) IsItCorrectUserID(userID int) bool {
	storager.mu.RLock()
	ok := storager.findUserID(userID)
	storager.mu.RUnlock()

	return ok
}

func (storager *Storager) findUserID(userID int) bool {
	for _, usedUserID := range storager.usedUserIDs {
		if usedUserID == userID {
			return true
		}
	}
	return false
}

func (storager *Storager) GetLastUserID(ctx context.Context) (int, error) {
	if storager.DB != nil {
		lastUserID, err := storager.DB.IncrementID(ctx)
		if err != nil {
			logger.Log.Fatal("Failed to read last user id from database", zap.Error(err))
			return lastUserID, err
		}

		storager.lastUserID = lastUserID
		return lastUserID, nil
	}

	storager.lastUserID = storager.lastUserID + 1
	return storager.lastUserID, nil
}

func (storager *Storager) SaveUserID(userID int) {
	storager.mu.Lock()
	storager.usedUserIDs = append(storager.usedUserIDs, userID)
	storager.mu.Unlock()
}

func (storager *Storager) DeleteByUserID(ctx context.Context, shortURLs []string, userID int) error {
	storager.mu.Lock()
	for _, shortURL := range shortURLs {
		originalSavedURL, ok := storager.findEntityByShortURL(shortURL)
		if ok {
			originalSavedURL.Deleted = true
			storager.URLMap[URLMapKey{shortURL, userID}] = originalSavedURL
		}
	}
	storager.mu.Unlock()

	if storager.DB != nil {
		err := storager.DB.UpdateDeletedSavedURLBatch(ctx, shortURLs, userID)
		return err
	} else if storager.isWithFile {
		// а что с файлом делать? Просто дописать?
		logger.Log.Info("Update file")
		filteredStore := []models.SavedURL{}

		storager.mu.RLock()
		for _, shortURL := range shortURLs {
			originalSavedURL, ok := storager.URLMap[URLMapKey{shortURL, userID}]
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
