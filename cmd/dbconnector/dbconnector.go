package dbconnector

import (
	"database/sql"

	"github.com/theheadmen/urlShort/cmd/logger"
	"github.com/theheadmen/urlShort/cmd/models"
	"go.uber.org/zap"

	_ "github.com/lib/pq"
)

type DBConnector struct {
	DB      *sql.DB
	IsAlive bool
}

func NewDBConnector(psqlInfo string) *DBConnector {
	// for local tests can be used "host=localhost port=5432 user=postgres password=example dbname=godb sslmode=disable"
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		logger.Log.Debug("Can't open DB", zap.String("error", err.Error()))
		return &DBConnector{
			DB:      db,
			IsAlive: false,
		}
	}
	//defer db.Close()

	err = db.Ping()
	if err != nil {
		logger.Log.Debug("Can't ping DB", zap.String("error", err.Error()))
		return &DBConnector{
			DB:      db,
			IsAlive: false,
		}
	}

	sqlStatement := `
	CREATE TABLE IF NOT EXISTS urls (
		id SERIAL PRIMARY KEY,
		shortURL VARCHAR(255),
		originalURL VARCHAR(255),
		UNIQUE(originalURL)
	);`
	_, err = db.Exec(sqlStatement)
	if err != nil {
		logger.Log.Debug("Can't create urls table", zap.String("error", err.Error()))
		return &DBConnector{
			DB:      db,
			IsAlive: false,
		}
	}

	return &DBConnector{
		DB:      db,
		IsAlive: true,
	}
}

func (dbConnector *DBConnector) InsertSavedURL(savedURL models.SavedURL) error {
	sqlStatement := `
	INSERT INTO urls (id, shortURL, originalURL)
	VALUES ($1, $2, $3)
	`

	_, err := dbConnector.DB.Exec(sqlStatement, savedURL.UUID, savedURL.ShortURL, savedURL.OriginalURL)
	if err != nil {
		logger.Log.Fatal("Failed to write in database", zap.Error(err))
	}

	logger.Log.Info("Write new data to database", zap.Int("UUID", savedURL.UUID), zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL))

	return err
}

func (dbConnector *DBConnector) SelectAllSavedURLs() ([]models.SavedURL, error) {
	var savedURLs []models.SavedURL
	var emptyURLs []models.SavedURL

	sqlStatement := `SELECT id, shortURL, originalURL FROM urls`
	rows, err := dbConnector.DB.Query(sqlStatement)
	if err != nil {
		logger.Log.Fatal("Failed to read from database", zap.Error(err))
		return emptyURLs, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL)
		if err != nil {
			logger.Log.Fatal("Failed to read from database", zap.Error(err))
			return emptyURLs, err
		}
		savedURLs = append(savedURLs, savedURL)
	}

	err = rows.Err()
	if err != nil {
		logger.Log.Fatal("Failed to read from database", zap.Error(err))
		return emptyURLs, err
	}

	return savedURLs, err
}
