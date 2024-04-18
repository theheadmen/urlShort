package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/theheadmen/urlShort/internal/dbconnector"
	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/serverapi"
	config "github.com/theheadmen/urlShort/internal/serverconfig"
	"github.com/theheadmen/urlShort/internal/storage"
	"github.com/theheadmen/urlShort/internal/storage/database"
	"github.com/theheadmen/urlShort/internal/storage/file"
	"go.uber.org/zap"
)

var (
	buildVersion string = "N/A"
	buildDate    string = "N/A"
	buildCommit  string = "N/A"
)

// для локального теста с бд
// go run . -a ":8081" -b "http://localhost:8081" -d "host=localhost port=5432 user=postgres password=example dbname=godb sslmode=disable"
func main() {
	fmt.Printf("Build version:=%s\n, Build date:=%s\n, Build commit:=%s\n", buildVersion, buildDate, buildCommit)

	configStore := config.NewConfigStore()
	configStore.ParseFlags()

	// создадим контекст который можно отменить
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := logger.Initialize(configStore.FlagLogLevel); err != nil {
		panic(err)
	}
	logger.Log.Info("Running server", zap.String("address", configStore.FlagRunAddr), zap.String("short address", configStore.FlagShortRunAddr), zap.String("file", configStore.FlagFile), zap.String("db", configStore.FlagDB))
	dbConnector, err := dbconnector.NewDBConnector(ctx, configStore.FlagDB)
	if err != nil {
		logger.Log.Debug("Can't open stable connection with DB", zap.String("error", err.Error()))
	}
	var storager storage.Storage
	if dbConnector != nil {
		storager = database.NewDatabaseStorage(make(map[storage.URLMapKey]models.SavedURL), dbConnector, ctx)
	} else {
		storager = file.NewFileStorage(configStore.FlagFile, true /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL), ctx)
	}

	router := serverapi.MakeChiServ(configStore, storager)

	server := &http.Server{
		Addr:    configStore.FlagRunAddr,
		Handler: router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Info("Server is down", zap.String("error", err.Error()))
		}
	}()

	// блокируем пока контекст не завершится, тем или иным путем
	<-ctx.Done()
}
