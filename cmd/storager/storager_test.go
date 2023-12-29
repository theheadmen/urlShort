package storager

import (
	"os"
	"sync"
	"testing"

	"github.com/theheadmen/urlShort/cmd/models"
)

func TestStoragerReadAllWriteFile(t *testing.T) {
	fname := `settings.json`
	storager := Storager{
		filePath:   fname,
		isWithFile: false,
		URLMap:     make(map[string]string),
		mu:         sync.RWMutex{},
	}
	savedURL := models.SavedURL{
		UUID:        1,
		ShortURL:    `ShortURL`,
		OriginalURL: `OriginalURL`,
	}
	if err := storager.Save(savedURL); err != nil {
		t.Error(err)
	}

	storager.ReadAllDataFromFile()
	originalURL, ok := storager.GetURL("ShortURL")
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
