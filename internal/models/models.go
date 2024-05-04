// Package models содержит определения структур данных, используемых в приложении.
package models

// Request представляет собой структуру для запроса URL.
type Request struct {
	URL string `json:"url"`
}

// Response представляет собой структуру для ответа с результатом обработки.
type Response struct {
	Result string `json:"result"`
}

// SavedURL представляет собой структуру для сохраненного URL.
type SavedURL struct {
	UUID        int    `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
	UserID      int    `json:"user_id"`
	Deleted     bool   `json:"deleted"`
}

// BatchRequest представляет собой структуру для пакетного запроса URL.
type BatchRequest struct {
	CorrelationID string `json:"correlation_id"`
	OriginalURL   string `json:"original_url"`
}

// BatchResponse представляет собой структуру для пакетного ответа с сокращенным URL.
type BatchResponse struct {
	CorrelationID string `json:"correlation_id"`
	ShortURL      string `json:"short_url"`
}

// BatchByUserIDResponse представляет собой структуру для пакетного ответа с URL, принадлежащих определенному пользователю.
type BatchByUserIDResponse struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

// StatsResponse представляет собой структуру для пакетного ответа с количеством URL и Users обработанных сервером.
type StatsResponse struct {
	URLs  int `json:"urls"`
	Users int `json:"users"`
}
