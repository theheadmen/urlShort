package config

import (
	"os"
	"testing"
)

func TestServerConfigReadWriteFile(t *testing.T) {
	fname := `settings.json`
	settings := SavedURL{
		UUID:        1,
		ShortURL:    `localhost`,
		OriginalURL: `localhost`,
	}
	if err := settings.Save(fname); err != nil {
		t.Error(err)
	}
	var result SavedURL
	if err := (&result).Load(fname); err != nil {
		t.Error(err)
	}
	if settings != result {
		t.Errorf(`%+v не равно %+v`, settings, result)
	}
	// удалим файл settings.json
	if err := os.Remove(fname); err != nil {
		t.Error(err)
	}
}

func TestServerConfigReadAllWriteFile(t *testing.T) {
	fname := `settings.json`
	settings := SavedURL{
		UUID:        1,
		ShortURL:    `ShortURL`,
		OriginalURL: `OriginalURL`,
	}
	if err := settings.Save(fname); err != nil {
		t.Error(err)
	}

	urlMap := ReadAllDataFromFile(fname)
	originalURL, ok := urlMap["ShortURL"]
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
