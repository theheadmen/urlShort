package dbconnector

import (
	"context"
	"database/sql"

	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	"go.uber.org/zap"

	"github.com/lib/pq"
)

// DBConnector представляет собой структуру для работы с базой данных.
type DBConnector struct {
	DB *sql.DB
}

// NewDBConnector создает новый экземпляр DBConnector и инициализирует подключение к базе данных.
// Если подключение не удается, возвращает ошибку.
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
		deleted BOOLEAN DEFAULT FALSE,
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

// InsertSavedURLBatch вставляет несколько URL в базу данных в рамках одной транзакции.
// Если транзакция не удается, возвращает ошибку.
func (dbConnector *DBConnector) InsertSavedURLBatch(ctx context.Context, savedURLs []models.SavedURL, userID int) error {
	tx, err := dbConnector.DB.BeginTx(ctx, nil)
	if err != nil {
		logger.Log.Error("Failed to initiate transaction for DB", zap.Error(err))
		return err
	}

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO urls(shortURL, originalURL, userID) VALUES($1, $2, $3)")
	if err != nil {
		logger.Log.Error("Failed to prepate query for DB", zap.Error(err))
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, savedURL := range savedURLs {
		_, err := stmt.ExecContext(ctx, savedURL.ShortURL, savedURL.OriginalURL, userID)
		if err != nil {
			tx.Rollback()
			logger.Log.Error("Failed to insert query for DB", zap.Error(err))
			return err
		}
		logger.Log.Info("Write new data to database", zap.String("OriginalURL", savedURL.OriginalURL), zap.String("ShortURL", savedURL.ShortURL), zap.Int("userID", userID))
	}

	err = tx.Commit()
	if err != nil {
		logger.Log.Error("Failed to commit transaction DB", zap.Error(err))
		return err
	}

	logger.Log.Info("Inserted new data to database", zap.Int("count", len(savedURLs)))

	return err
}

// SelectAllSavedURLs возвращает все сохраненные URL из базы данных.
// Если чтение не удается, возвращает ошибку.
func (dbConnector *DBConnector) SelectAllSavedURLs(ctx context.Context) ([]models.SavedURL, error) {
	var savedURLs []models.SavedURL

	sqlStatement := `SELECT id, shortURL, originalURL, userID, deleted FROM urls`
	rows, err := dbConnector.DB.QueryContext(ctx, sqlStatement)
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL, &savedURL.UserID, &savedURL.Deleted)
		if err != nil {
			logger.Log.Error("Failed to read from database", zap.Error(err))
			return nil, err
		}
		savedURLs = append(savedURLs, savedURL)
	}

	err = rows.Err()
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}

	return savedURLs, err
}

// SelectSavedURLsForUserID возвращает все сохраненные URL для определенного пользователя.
// Если чтение не удается, возвращает ошибку.
func (dbConnector *DBConnector) SelectSavedURLsForUserID(ctx context.Context, userID int) ([]models.SavedURL, error) {
	var savedURLs []models.SavedURL

	sqlStatement := `SELECT id, shortURL, originalURL, userID, deleted FROM urls where userID = $1`
	rows, err := dbConnector.DB.QueryContext(ctx, sqlStatement, userID)
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL, &savedURL.UserID, &savedURL.Deleted)
		if err != nil {
			logger.Log.Error("Failed to read from database", zap.Error(err))
			return nil, err
		}
		savedURLs = append(savedURLs, savedURL)
	}

	err = rows.Err()
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}

	return savedURLs, err
}

// SelectSavedURLsForUserID возвращает все сохраненные URL для определенного URL.
// Если чтение не удается, возвращает ошибку.
func (dbConnector *DBConnector) SelectSavedURLsForShortURL(ctx context.Context, shortURL string) ([]models.SavedURL, error) {
	var savedURLs []models.SavedURL

	sqlStatement := `SELECT id, shortURL, originalURL, userID, deleted FROM urls where shortURL = $1`
	rows, err := dbConnector.DB.QueryContext(ctx, sqlStatement, shortURL)
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL, &savedURL.UserID, &savedURL.Deleted)
		if err != nil {
			logger.Log.Error("Failed to read from database", zap.Error(err))
			return nil, err
		}
		savedURLs = append(savedURLs, savedURL)
	}

	err = rows.Err()
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}

	return savedURLs, err
}

// SelectSavedURLsForShortURL возвращает все сохраненные URL для определенного короткого URL.
// Если чтение не удается, возвращает ошибку.
func (dbConnector *DBConnector) SelectSavedURLsForShortURLAndUserID(ctx context.Context, shortURL string, userID int) ([]models.SavedURL, error) {
	var savedURLs []models.SavedURL

	sqlStatement := `SELECT id, shortURL, originalURL, userID, deleted FROM urls where shortURL = $1 AND userID = $2`
	rows, err := dbConnector.DB.QueryContext(ctx, sqlStatement, shortURL, userID)
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var savedURL models.SavedURL
		err = rows.Scan(&savedURL.UUID, &savedURL.ShortURL, &savedURL.OriginalURL, &savedURL.UserID, &savedURL.Deleted)
		if err != nil {
			logger.Log.Error("Failed to read from database", zap.Error(err))
			return nil, err
		}
		savedURLs = append(savedURLs, savedURL)
	}

	err = rows.Err()
	if err != nil {
		logger.Log.Error("Failed to read from database", zap.Error(err))
		return nil, err
	}

	return savedURLs, err
}

// IncrementID увеличивает значение на 1 и возвращает новое значение и ошибку.
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

// UpdateDeletedSavedURLBatch обновляет несколько URL в базе данных в рамках одной транзакции, помечая их как удаленные.
// Если транзакция не удается, возвращает ошибку.
func (dbConnector *DBConnector) UpdateDeletedSavedURLBatch(ctx context.Context, shortURLs []string, userID int) error {
	stmt, err := dbConnector.DB.PrepareContext(ctx, `
		UPDATE urls
		SET deleted = TRUE
		WHERE shortURL = ANY($1)
		AND userID = $2;
	`)
	if err != nil {
		logger.Log.Error("Failed to prepare the statement: ", zap.Error(err))
		return err
	}
	defer stmt.Close()

	// Execute the statement
	res, err := stmt.ExecContext(ctx, pq.Array(shortURLs), userID)
	if err != nil {
		logger.Log.Error("Failed to execute the statement: ", zap.Error(err))
		return err
	}

	// Check how many rows were affected
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		logger.Log.Error("Failed to get the number of rows affected: ", zap.Error(err))
		return err
	}

	logger.Log.Info("Inserted new data to database", zap.Int64("count", rowsAffected))

	return nil
}
