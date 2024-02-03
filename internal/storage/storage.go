package storage

import (
	"context"

	"github.com/theheadmen/urlShort/internal/models"
)

type URLMapKey struct {
	ShortURL string
	UserID   int
}

type Storage interface {
	ReadAllData(ctx context.Context) error
	ReadAllDataForUserID(ctx context.Context, userID int) ([]models.SavedURL, error)
	StoreURL(ctx context.Context, shortURL string, originalURL string, userID int) bool
	StoreURLBatch(ctx context.Context, forStore []models.SavedURL, userID int)
	GetLastUserID(ctx context.Context) (int, error)
	DeleteByUserID(ctx context.Context, shortURLs []string, userID int) error
	GetURLForAnyUserID(shortURL string) (models.SavedURL, bool)
	IsItCorrectUserID(userID int) bool
	SaveUserID(userID int)
	PingContext(ctx context.Context) error
}
