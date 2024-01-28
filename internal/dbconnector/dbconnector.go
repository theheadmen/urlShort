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
		userID INT,
		UNIQUE(originalURL, userID)
	);
	CREATE TABLE IF NOT EXISTS last_user_id (
		id INT PRIMARY KEY DEFAULT 1
	);
	INSERT INTO last_user_id (id) VALUES (1) ON CONFLICT DO NOTHING;`
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

func (dbConnector *DBConnector) InsertSavedURLBatch(ctx context.Context, savedURLs []models.SavedURL, userID int) error {
	tx, err := dbConnector.DB.BeginTx(ctx, nil)
	if err != nil {
		logger.Log.Info("Failed to initiate transaction for DB", zap.Error(err))
		return err
	}

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO urls(shortURL, originalURL, userID) VALUES($1, $2, $3)")
	if err != nil {
		logger.Log.Info("Failed to prepate query for DB", zap.Error(err))
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, savedURL := range savedURLs {
		_, err := stmt.ExecContext(ctx, savedURL.ShortURL, savedURL.OriginalURL, userID)
		if err != nil {
			tx.Rollback()
			logger.Log.Info("Failed to insert query for DB", zap.Error(err))
			return err
		}
		logger.Log.Info("Write new data to database", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("userID", userID))
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

	sqlStatement := `SELECT id, shortURL, originalURL, userID FROM urls`
	rows, err := dbConnector.DB.QueryContext(ctx, sqlStatement)
	if err != nil {
		logger.Log.Info("Failed to read from database", zap.Error(err))
		return emptyURLs, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL, &savedURL.UserID)
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

func (dbConnector *DBConnector) SelectSavedURLsForUserID(ctx context.Context, userID int) ([]models.SavedURL, error) {
	var savedURLs []models.SavedURL
	var emptyURLs []models.SavedURL

	sqlStatement := `SELECT id, shortURL, originalURL, userID FROM urls where userID = $1`
	rows, err := dbConnector.DB.QueryContext(ctx, sqlStatement, userID)
	if err != nil {
		logger.Log.Info("Failed to read from database", zap.Error(err))
		return emptyURLs, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL, &savedURL.UserID)
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

// GetOrInsertID checks if the table is empty and inserts a default value if it is.
func (dbConnector *DBConnector) GetOrInsertID(ctx context.Context) (int, error) {
	// Insert a default value if the table is empty
	_, err := dbConnector.DB.ExecContext(ctx, "INSERT INTO last_user_id (id) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM last_user_id)")
	if err != nil {
		return 0, err
	}

	// Retrieve the id
	var id int
	err = dbConnector.DB.QueryRowContext(ctx, "SELECT id FROM last_user_id").Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// IncrementID increments the value in the table by 1 and returns the new value.
func (dbConnector *DBConnector) IncrementID(ctx context.Context) (int, error) {
	var newID int
	err := dbConnector.DB.QueryRowContext(ctx, `
		WITH updated AS (
			UPDATE last_user_id
			SET id = id + 1
			RETURNING id
		)
		SELECT id FROM updated
		UNION ALL
		SELECT id FROM last_user_id WHERE NOT EXISTS (SELECT 1 FROM updated)
	`).Scan(&newID)
	if err != nil {
		return 0, err
	}

	return newID, nil
}
