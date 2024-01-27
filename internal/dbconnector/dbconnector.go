package dbconnector

import (
	"context"
	"database/sql"

	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	"go.uber.org/zap"

	_ "github.com/lib/pq"
)

type DBConnector struct {
	DB *sql.DB
}

func NewDBConnector(ctx context.Context, psqlInfo string) (*DBConnector, error) {
	// for local tests can be used "host=localhost port=5432 user=postgres password=example dbname=godb sslmode=disable"
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		logger.Log.Debug("Can't open DB", zap.String("error", err.Error()))
		return nil, err
	}
	//defer db.Close()

	err = db.PingContext(ctx)
	if err != nil {
		logger.Log.Debug("Can't ping DB", zap.String("error", err.Error()))
		db.Close() // Close the database connection if ping fails.
		return nil, err
	}

	sqlStatement := `
	CREATE TABLE IF NOT EXISTS urls (
		id SERIAL PRIMARY KEY,
		shortURL VARCHAR(255),
		originalURL VARCHAR(255),
		UNIQUE(originalURL)
	);`
	_, err = db.ExecContext(ctx, sqlStatement)
	if err != nil {
		logger.Log.Debug("Can't create urls table", zap.String("error", err.Error()))
		db.Close() // Close the database connection if table creation fails.
		return nil, err
	}

	return &DBConnector{
		DB: db,
	}, nil
}

func (dbConnector *DBConnector) InsertSavedURLBatch(ctx context.Context, savedURLs []models.SavedURL) error {
	tx, err := dbConnector.DB.BeginTx(ctx, nil)
	if err != nil {
		logger.Log.Info("Failed to initiate transaction for DB", zap.Error(err))
		return err
	}

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO urls(shortURL, originalURL) VALUES($1, $2)")
	if err != nil {
		logger.Log.Info("Failed to prepate query for DB", zap.Error(err))
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, savedURL := range savedURLs {
		_, err := stmt.ExecContext(ctx, savedURL.ShortURL, savedURL.OriginalURL)
		if err != nil {
			tx.Rollback()
			logger.Log.Info("Failed to insert query for DB", zap.Error(err))
			return err
		}
		logger.Log.Info("Write new data to database", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL))
	}

	err = tx.Commit()
	if err != nil {
		logger.Log.Info("Failed to commit transaction DB", zap.Error(err))
		return err
	}

	logger.Log.Info("Inserted new data to database", zap.Int("count", len(savedURLs)))

	return err
}

func (dbConnector *DBConnector) SelectAllSavedURLs(ctx context.Context) ([]models.SavedURL, error) {
	var savedURLs []models.SavedURL
	var emptyURLs []models.SavedURL

	sqlStatement := `SELECT id, shortURL, originalURL FROM urls`
	rows, err := dbConnector.DB.QueryContext(ctx, sqlStatement)
	if err != nil {
		logger.Log.Info("Failed to read from database", zap.Error(err))
		return emptyURLs, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL)
		if err != nil {
			logger.Log.Info("Failed to read from database", zap.Error(err))
			return emptyURLs, err
		}
		savedURLs = append(savedURLs, savedURL)
	}

	err = rows.Err()
	if err != nil {
		logger.Log.Info("Failed to read from database", zap.Error(err))
		return emptyURLs, err
	}

	return savedURLs, err
}
