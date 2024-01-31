package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/theheadmen/urlShort/internal/dbconnector"
	"github.com/theheadmen/urlShort/internal/logger"
	"github.com/theheadmen/urlShort/internal/models"
	"github.com/theheadmen/urlShort/internal/serverapi"
	config "github.com/theheadmen/urlShort/internal/serverconfig"
	"github.com/theheadmen/urlShort/internal/storager"
	"go.uber.org/zap"

	_ "github.com/lib/pq"
)

// for test
// go run . -a ":8081" -b "http://localhost:8081" -d "host=localhost port=5432 user=postgres password=example dbname=godb sslmode=disable"
func main() {
	configStore := config.NewConfigStore()
	configStore.ParseFlags()

	// Create a context that can be cancelled
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
	storager := storager.NewStorager(configStore.FlagFile, true /*isWithFile*/, make(map[storager.URLMapKey]models.SavedURL), dbConnector, ctx)

	// Create a new chi router
	router := serverapi.MakeChiServ(configStore, storager)

	// Create a new server
	server := &http.Server{
		Addr:    configStore.FlagRunAddr,
		Handler: router,
	}

	// Start the server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal("Server is down", zap.String("error", err.Error()))
		}
	}()

	// Block until we receive a signal or the context is cancelled
	<-ctx.Done()
}
