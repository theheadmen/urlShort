package file

import (
	"context"
	"os"
	"testing"

	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/storage"
)

func TestStoragerReadAllWriteFile(t *testing.T) {
	fname := `settings.json`
	userID := 1
	ctx := context.Background()
	storager := NewFileStorage(fname, false, make(map[storage.URLMapKey]models.SavedURL), ctx)
	savedURL := models.SavedURL{
		UUID:        1,
		ShortURL:    `ShortURL`,
		OriginalURL: `OriginalURL`,
		UserID:      userID,
		Deleted:     false,
	}
	if err := storager.Save(savedURL); err != nil {
		t.Error(err)
	}

	if err := storager.ReadAllData(ctx); err != nil {
		t.Error(err)
	}
	originalURL, ok := storager.GetURL("ShortURL", userID)
	if !ok {
		t.Errorf(`Не нашли url для %+s`, "ShortURL")
	}

	if originalURL != "OriginalURL" {
		t.Errorf(`%+s не равно %+s`, originalURL, "OriginalURL")
	}
	// удалим файл settings.json
	if err := os.Remove(fname); err != nil {
		t.Error(err)
	}
}
