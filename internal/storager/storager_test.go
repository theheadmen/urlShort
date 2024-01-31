package storager

import (
	"os"
	"sync"
	"testing"

	"github.com/theheadmen/urlShort/internal/models"
)

func TestStoragerReadAllWriteFile(t *testing.T) {
	fname := `settings.json`
	userID := 1
	storager := Storager{
		filePath:   fname,
		isWithFile: false,
		URLMap:     make(map[URLMapKey]models.SavedURL),
		mu:         sync.RWMutex{},
		DB:         nil,
		lastUserID: userID,
	}
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

	if err := storager.ReadAllDataFromFile(); err != nil {
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
