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

func NewDBConnectorForTest() *DBConnector {
	return &DBConnector{
		DB:      nil,
		IsAlive: false,
	}
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

func (dbConnector *DBConnector) InsertSavedURLBatch(savedURLs []models.SavedURL) error {
	tx, err := dbConnector.DB.Begin()
	if err != nil {
		logger.Log.Fatal("Failed to initiate transaction for DB", zap.Error(err))
	}

	stmt, err := tx.Prepare("INSERT INTO urls(shortURL, originalURL) VALUES($1, $2)")
	if err != nil {
		logger.Log.Fatal("Failed to prepate query for DB", zap.Error(err))
	}
	defer stmt.Close()

	for _, savedURL := range savedURLs {
		_, err := stmt.Exec(savedURL.ShortURL, savedURL.OriginalURL)
		if err != nil {
			tx.Rollback()
			logger.Log.Fatal("Failed to insert query for DB", zap.Error(err))
		}
		logger.Log.Info("Write new data to database", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL))
	}

	err = tx.Commit()
	if err != nil {
		logger.Log.Fatal("Failed to commit transaction DB", zap.Error(err))
	}

	logger.Log.Info("Inserted new data to database", zap.Int("count", len(savedURLs)))

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
