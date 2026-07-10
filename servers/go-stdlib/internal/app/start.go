package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"shared/database"
	"syscall"
	"time"
)

func (app *App) Start() error {
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.env.HOST, app.env.PORT),
		Handler:      app.handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Stdlib Server development: http://%s", server.Addr)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		log.Printf("Server error: %v", err)
		return err
	}

	log.Println("\nShutting down gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	// Close the DB pools on both paths — a Shutdown timeout must not leak them —
	// but always after Shutdown returns, so in-flight requests keep their database.
	shutdownErr := server.Shutdown(shutdownCtx)
	database.DisconnectConnections()
	if shutdownErr != nil {
		log.Printf("Server shutdown error: %v", shutdownErr)
		return shutdownErr
	}

	log.Println("Server stopped.")
	return nil
}
