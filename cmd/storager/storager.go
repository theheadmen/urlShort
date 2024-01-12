package storager

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"

	"github.com/theheadmen/urlShort/cmd/logger"
	"github.com/theheadmen/urlShort/cmd/models"
	"go.uber.org/zap"
)

type Storager struct {
	filePath   string
	isWithFile bool
	URLMap     map[string]string
	mu         sync.RWMutex
}

func NewStorager(filePath string, isWithFile bool, URLMap map[string]string) *Storager {
	return &Storager{
		filePath:   filePath,
		isWithFile: isWithFile,
		URLMap:     URLMap,
		mu:         sync.RWMutex{},
	}
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
	for scanner.Scan() {
		var result models.SavedURL
		err := json.Unmarshal([]byte(scanner.Text()), &result)
		if err != nil {
			logger.Log.Debug("Failed unmarshal data", zap.Error(err))
		}
		storager.URLMap[result.ShortURL] = result.OriginalURL
		logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL))
	}

	if err := scanner.Err(); err != nil {
		logger.Log.Fatal("Failed to read file", zap.Error(err))
	}

	return err
}

func (storager *Storager) StoreURL(shortURL string, originalURL string) {
	_, ok := storager.GetURL(shortURL)

	if ok {
		logger.Log.Info("We already have data for this url", zap.String("OriginalURL", originalURL), zap.String("ShortURL", shortURL))
		return
	}

	storager.mu.Lock()
	storager.URLMap[shortURL] = originalURL
	storager.mu.Unlock()

	if storager.isWithFile {
		savedURL := models.SavedURL{
			UUID:        len(storager.URLMap),
			ShortURL:    shortURL,
			OriginalURL: originalURL,
		}
		storager.Save(savedURL)
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
	logger.Log.Info("Write new data to file", zap.Int("UUID", savedURL.UUID), zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL))
	return nil
}

func (storager *Storager) GetURL(shortURL string) (string, bool) {
	storager.mu.RLock()
	originalURL, ok := storager.URLMap[shortURL]
	storager.mu.RUnlock()

	return originalURL, ok
}
