// Package config содержит определения структур используемых в приложении для конфигурации
package config

import (
	"encoding/json"
	"flag"
	"os"
)

// ConfigStore структура с всеми используемыми флагами
type ConfigStore struct {
	FlagRunAddr      string `json:"server_address"`
	FlagShortRunAddr string `json:"base_url"`
	FlagLogLevel     string `json:"-"`
	FlagFile         string `json:"file_storage_path"`
	FlagDB           string `json:"database_dsn"`
	FlagLTS          bool   `json:"enable_https"`
	FlagConfig       string `json:"-"`
}

// NewConfigStore возвращает ConfigStore с пустыми значениями всех флагов
func NewConfigStore() *ConfigStore {
	return &ConfigStore{
		FlagRunAddr:      "",
		FlagShortRunAddr: "",
		FlagLogLevel:     "",
		FlagFile:         "",
		FlagDB:           "",
		FlagLTS:          false,
		FlagConfig:       "",
	}
}

// readConfigFile читает файл конфигурации и возвращает временный конфиг
func (configStore *ConfigStore) readConfigFile() *ConfigStore {
	data, err := os.ReadFile(configStore.FlagConfig)
	if err != nil {
		return nil
	}

	tempConfig := NewConfigStore()
	err = json.Unmarshal(data, tempConfig)
	if err != nil {
		return nil
	}

	return tempConfig
}

// parseFlags обрабатывает аргументы командной строки
// и сохраняет их значения в соответствующих переменных
func (configStore *ConfigStore) ParseFlags() {
	flagRunAddrDef := ":8080"
	flagShortRunAddrDef := "http://localhost:8080"
	flagFileDef := "/tmp/short-url-db.json"
	flagDBDef := ""

	flag.StringVar(&configStore.FlagRunAddr, "a", flagRunAddrDef, "address and port to run server")
	flag.StringVar(&configStore.FlagShortRunAddr, "b", flagShortRunAddrDef, "address and port to return short url")
	flag.StringVar(&configStore.FlagLogLevel, "l", "debug", "log level")
	flag.StringVar(&configStore.FlagFile, "f", flagFileDef, "file with saved urls")
	flag.StringVar(&configStore.FlagDB, "d", flagDBDef, "params to connect with DB")
	flag.BoolVar(&configStore.FlagLTS, "s", false, "use LTS")
	flag.StringVar(&configStore.FlagConfig, "c", "", "path to config file")
	flag.StringVar(&configStore.FlagConfig, "config", "", "path to config file")
	// парсим переданные серверу аргументы в зарегистрированные переменные
	flag.Parse()

	if envConfig := os.Getenv("CONFIG"); envConfig != "" {
		configStore.FlagConfig = envConfig
	}

	// Проверяем наличие файла конфигурации и читаем его
	if configStore.FlagConfig != "" {
		tempConfig := configStore.readConfigFile()
		// если какое-то значение не было выставлено как флаг ранее - используем значение из конфига-файла
		if configStore.FlagRunAddr == flagRunAddrDef {
			configStore.FlagRunAddr = tempConfig.FlagRunAddr
		}
		if configStore.FlagShortRunAddr == flagShortRunAddrDef {
			configStore.FlagShortRunAddr = tempConfig.FlagShortRunAddr
		}
		if configStore.FlagFile == flagFileDef {
			configStore.FlagFile = tempConfig.FlagFile
		}
		if configStore.FlagDB == flagDBDef {
			configStore.FlagDB = tempConfig.FlagDB
		}
		if !configStore.FlagLTS {
			configStore.FlagLTS = tempConfig.FlagLTS
		}
	}

	// а затем в любом случае смотрим еще и переменные окружения
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

	if envDB := os.Getenv("DATABASE_DSN"); envDB != "" {
		configStore.FlagDB = envDB
	}
}
