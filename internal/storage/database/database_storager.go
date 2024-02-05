package database

import (
	"context"
	"sync"

	"github.com/theheadmen/urlShort/internal/dbconnector"
	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/storage"
	"go.uber.org/zap"
)

type DatabaseStorage struct {
	URLMap      map[storage.URLMapKey]models.SavedURL
	mu          sync.RWMutex
	DB          *dbconnector.DBConnector
	lastUserID  int
	usedUserIDs []int
}

func NewDatabaseStorage(URLMap map[storage.URLMapKey]models.SavedURL, dbConnector *dbconnector.DBConnector, ctx context.Context) *DatabaseStorage {
	var empty []int

	storager := &DatabaseStorage{
		URLMap:      URLMap,
		mu:          sync.RWMutex{},
		DB:          dbConnector,
		lastUserID:  0,
		usedUserIDs: empty,
	}
	err := storager.ReadAllData(ctx)
	if err != nil {
		logger.Log.Error("Failed to read data", zap.Error(err))
	}
	return storager
}

func (storager *DatabaseStorage) ReadAllData(ctx context.Context) error {
	urls, err := storager.DB.SelectAllSavedURLs(ctx)
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return err
	}

	for _, url := range urls {
		storager.usedUserIDs = append(storager.usedUserIDs, url.UserID)
		logger.Log.Info("Read new data from database", zap.Int("UUID", url.UUID), zap.String("OriginalURL", url.OriginalURL), zap.String("ShortURL", url.ShortURL), zap.Int("UserID", url.UserID), zap.Bool("Deleted", url.Deleted))
	}

	return err
}

func (storager *DatabaseStorage) ReadAllDataForUserID(ctx context.Context, userID int) ([]models.SavedURL, error) {
	urls, err := storager.DB.SelectSavedURLsForUserID(ctx, userID)
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return []models.SavedURL{}, err
	}

	return urls, err
}

// возвращает true если это значение уже было записано ранее
func (storager *DatabaseStorage) StoreURL(ctx context.Context, shortURL string, originalURL string, userID int) (bool, error) {
	_, ok, err := storager.GetURL(ctx, shortURL, userID)
	if err != nil {
		return false, err
	}

	if ok {
		logger.Log.Info("We already have data for this url", zap.String("OriginalURL", originalURL), zap.String("ShortURL", shortURL), zap.Bool("Deleted", false))
		return true, nil
	}

	savedURL := models.SavedURL{
		UUID:        0,
		ShortURL:    shortURL,
		OriginalURL: originalURL,
		UserID:      userID,
		Deleted:     false,
	}

	err = storager.DB.InsertSavedURLBatch(ctx, []models.SavedURL{savedURL}, userID)

	return false, err
}

func (storager *DatabaseStorage) StoreURLBatch(ctx context.Context, forStore []models.SavedURL, userID int) error {
	var filteredStore []models.SavedURL
	for _, savedURL := range forStore {
		_, ok, err := storager.GetURL(ctx, savedURL.ShortURL, userID)
		if err != nil {
			return err
		}

		if ok {
			logger.Log.Info("We already have data for this url", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("UserID", userID), zap.Bool("Deleted", savedURL.Deleted))
		} else {
			filteredStore = append(filteredStore, savedURL)
		}
	}
	// если у нас уже все и так было вставлено, нам не нужно ничего сохранять
	if len(filteredStore) != 0 {
		err := storager.DB.InsertSavedURLBatch(ctx, filteredStore, userID)
		return err
	}

	return nil
}

func (storager *DatabaseStorage) GetURL(ctx context.Context, shortURL string, userID int) (string, bool, error) {
	savedURLs, err := storager.DB.SelectSavedURLsForShortURLAndUserId(ctx, shortURL, userID)
	if err != nil {
		return "", false, err
	}

	if len(savedURLs) == 0 {
		return "", false, nil
	} else {
		// в теории и должно быть максимум одно значение, но для простоты используем массив
		return savedURLs[0].OriginalURL, true, nil
	}
}

func (storager *DatabaseStorage) GetURLForAnyUserID(ctx context.Context, shortURL string) (models.SavedURL, bool, error) {
	savedURLs, err := storager.DB.SelectSavedURLsForShortURL(ctx, shortURL)
	if err != nil {
		return models.SavedURL{}, false, err
	}

	if len(savedURLs) == 0 {
		return models.SavedURL{}, false, nil
	} else {
		// в теории и должно быть максимум одно значение, но для простоты используем массив
		return savedURLs[0], true, nil
	}
}

func (storager *DatabaseStorage) IsItCorrectUserID(userID int) bool {
	storager.mu.RLock()
	ok := storager.findUserID(userID)
	storager.mu.RUnlock()

	return ok
}

func (storager *DatabaseStorage) findUserID(userID int) bool {
	for _, usedUserID := range storager.usedUserIDs {
		if usedUserID == userID {
			return true
		}
	}
	return false
}

func (storager *DatabaseStorage) GetLastUserID(ctx context.Context) (int, error) {
	lastUserID, err := storager.DB.IncrementID(ctx)
	if err != nil {
		logger.Log.Error("Failed to read last user id from database", zap.Error(err))
		return lastUserID, err
	}

	storager.lastUserID = lastUserID
	return lastUserID, nil
}

func (storager *DatabaseStorage) SaveUserID(userID int) {
	storager.mu.Lock()
	storager.usedUserIDs = append(storager.usedUserIDs, userID)
	storager.mu.Unlock()
}

func (storager *DatabaseStorage) DeleteByUserID(ctx context.Context, shortURLs []string, userID int) error {
	err := storager.DB.UpdateDeletedSavedURLBatch(ctx, shortURLs, userID)
	return err
}

func (storager *DatabaseStorage) PingContext(ctx context.Context) error {
	err := storager.DB.DB.PingContext(ctx)
	if err != nil {
		logger.Log.Info("Can't ping DB", zap.String("error", err.Error()))
	}
	return err
}
