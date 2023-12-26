package config

import (
	"bufio"
	"encoding/json"
	"flag"
	"os"

	"github.com/theheadmen/urlShort/cmd/logger"
	"go.uber.org/zap"
)

type ConfigStore struct {
	FlagRunAddr      string
	FlagShortRunAddr string
	FlagLogLevel     string
	FlagFile         string
}

type SavedUrl struct {
	UUID        int    `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

// Save сохраняет настройки в файле fname.
func (savedUrl SavedUrl) Save(fname string) error {
	savedUrlJson, err := json.Marshal(savedUrl)
	if err != nil {
		logger.Log.Fatal("Failed to marshal new data", zap.Error(err))
		return err
	}
	file, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Log.Fatal("Failed to open file for writing", zap.Error(err))
		return err
	}
	defer file.Close()

	savedUrlJson = append(savedUrlJson, '\n')
	if _, err := file.Write(savedUrlJson); err != nil {
		logger.Log.Fatal("Failed to write to file", zap.Error(err))
		return err
	}
	logger.Log.Info("Write new data to file", zap.Int("UUID", savedUrl.UUID), zap.String("OriginalURL", savedUrl.OriginalURL), zap.String("ShortURL", savedUrl.ShortURL))
	return nil
}

// Load читает настройки из файла fname.
func (savedUrl *SavedUrl) Load(fname string) error {
	data, err := os.ReadFile(fname)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, savedUrl)
}

func ReadAllDataFromFile(filePath string) map[string]string {
	urlMap := make(map[string]string)

	// Read from file
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("File does not exist. Leaving savedUrls empty.")
		} else {
			logger.Log.Fatal("Failed to open file", zap.Error(err))
		}
	} else {
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var result SavedUrl
			err := json.Unmarshal([]byte(scanner.Text()), &result)
			if err != nil {
				logger.Log.Fatal("Failed unmarshal data", zap.Error(err))
			}
			urlMap[result.ShortURL] = result.OriginalURL
			logger.Log.Info("Read new data from file", zap.Int("UUID", result.UUID), zap.String("OriginalURL", result.OriginalURL), zap.String("ShortURL", result.ShortURL))
		}

		if err := scanner.Err(); err != nil {
			logger.Log.Fatal("Failed to read file", zap.Error(err))
		}
	}

	return urlMap
}

func NewConfigStore() *ConfigStore {
	return &ConfigStore{
		FlagRunAddr:      "",
		FlagShortRunAddr: "",
		FlagLogLevel:     "",
		FlagFile:         "",
	}
}

// parseFlags обрабатывает аргументы командной строки
// и сохраняет их значения в соответствующих переменных
func (configStore *ConfigStore) ParseFlags() {
	// регистрируем переменную flagRunAddr
	// как аргумент -a со значением :8080 по умолчанию
	flag.StringVar(&configStore.FlagRunAddr, "a", ":8080", "address and port to run server")
	flag.StringVar(&configStore.FlagShortRunAddr, "b", "http://localhost:8080", "address and port to return short url")
	flag.StringVar(&configStore.FlagLogLevel, "l", "debug", "log level")
	flag.StringVar(&configStore.FlagFile, "f", "./tmp/short-url-db.json", "file with saved urls")
	// парсим переданные серверу аргументы в зарегистрированные переменные
	flag.Parse()

	if envRunAddr := os.Getenv("SERVER_ADDRESS"); envRunAddr != "" {
		configStore.FlagRunAddr = envRunAddr
	}

	if envShortRunAddr := os.Getenv("BASE_URL"); envShortRunAddr != "" {
		configStore.FlagShortRunAddr = envShortRunAddr
	}

	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
		configStore.FlagLogLevel = envLogLevel
	}

	if envFile := os.Getenv("FILE_STORAGE_PATH"); envFile != "" {
		configStore.FlagFile = envFile
	}
}
