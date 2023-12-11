package config

import (
	"flag"
	"os"
)

type ConfigStore struct {
	FlagRunAddr      string
	FlagShortRunAddr string
}

func NewConfigStore() *ConfigStore {
	return &ConfigStore{
		FlagRunAddr:      "",
		FlagShortRunAddr: "",
	}
}

// parseFlags обрабатывает аргументы командной строки
// и сохраняет их значения в соответствующих переменных
func (configStore *ConfigStore) ParseFlags() {
	// регистрируем переменную flagRunAddr
	// как аргумент -a со значением :8080 по умолчанию
	flag.StringVar(&configStore.FlagRunAddr, "a", ":8080", "address and port to run server")
	flag.StringVar(&configStore.FlagShortRunAddr, "b", "http://localhost:8080", "address and port to return short url")
	// парсим переданные серверу аргументы в зарегистрированные переменные
	flag.Parse()

	if envRunAddr := os.Getenv("SERVER_ADDRESS"); envRunAddr != "" {
		configStore.FlagRunAddr = envRunAddr
	}

	if envShortRunAddr := os.Getenv("BASE_URL"); envShortRunAddr != "" {
		configStore.FlagShortRunAddr = envShortRunAddr
	}
}
