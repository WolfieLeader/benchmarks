package app

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"fiber-server/internal/database"
)

func (app *App) Start() error {
	addr := fmt.Sprintf("%s:%d", app.env.HOST, app.env.PORT)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("Fiber Server development: http://%s\n\n", addr)
		errCh <- app.router.Listen(addr)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Printf("Server error: %v", err)
			return err
		}
	}

	log.Println("\nShutting down gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	// Close the DB pools on both paths — a Shutdown timeout must not leak them —
	// but always after Shutdown returns, so in-flight requests keep their database.
	shutdownErr := app.router.ShutdownWithContext(shutdownCtx)
	database.DisconnectConnections()
	if shutdownErr != nil {
		log.Printf("Server shutdown error: %v", shutdownErr)
		return shutdownErr
	}

	log.Println("Server stopped.")
	return nil
}
