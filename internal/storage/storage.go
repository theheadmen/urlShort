// Package storage предоставляет интерфейс Storage для работы с хранилищем данных.
package storage

import (
	"context"

	"github.com/theheadmen/urlShort/internal/models"
)

// URLMapKey представляет собой структуру для ключа URL в хранилище.
type URLMapKey struct {
	ShortURL string // Сокращенный URL
	UserID   int    // Идентификатор пользователя
}

// Storage определяет интерфейс для работы с хранилищем данных.
type Storage interface {
	// ReadAllData читает все данные из хранилища.
	ReadAllData(ctx context.Context) error

	// ReadAllDataForUserID читает все данные для определенного пользователя из хранилища.
	ReadAllDataForUserID(ctx context.Context, userID int) ([]models.SavedURL, error)

	// StoreURL сохраняет URL в хранилище.
	StoreURL(ctx context.Context, shortURL string, originalURL string, userID int) (bool, error)

	// StoreURLBatch сохраняет несколько URL в хранилище.
	StoreURLBatch(ctx context.Context, forStore []models.SavedURL, userID int) error

	// GetLastUserID получает последний использованный идентификатор пользователя.
	GetLastUserID(ctx context.Context) (int, error)

	// DeleteByUserID удаляет URL, принадлежащие определенному пользователю.
	DeleteByUserID(ctx context.Context, shortURLs []string, userID int) error

	// GetURLForAnyUserID получает URL, независимо от пользователя.
	GetURLForAnyUserID(ctx context.Context, shortURL string) (models.SavedURL, bool, error)

	// IsItCorrectUserID проверяет, является ли идентификатор пользователя корректным.
	IsItCorrectUserID(userID int) bool

	// SaveUserID сохраняет идентификатор пользователя.
	SaveUserID(userID int)

	// PingContext проверяет соединение с хранилищем.
	PingContext(ctx context.Context) error

	// GetStats возвращает данные URLs и Users если запрос отправляется из доверяемой сети
	GetStats(ctx context.Context) (models.StatsResponse, error)
}
