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
	e := app.echo
	// echo drives its own *http.Server (e.Server); set the same core timeouts as the
	// other net/http-based Go servers before Start reads it. These are not perfectly
	// uniform across frameworks: chi and go-stdlib set exactly these three
	// (Read/Write/Idle); gin additionally sets ReadHeaderTimeout; fiber runs on
	// fasthttp and configures none of them. ReadHeaderTimeout is omitted here to match
	// chi/go-stdlib. Safe to set here: the go statement below happens-before the
	// listener goroutine.
	e.Server.ReadTimeout = 15 * time.Second
	e.Server.WriteTimeout = 15 * time.Second
	e.Server.IdleTimeout = 60 * time.Second

	addr := fmt.Sprintf("%s:%d", app.env.HOST, app.env.PORT)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Echo Server development: http://%s", addr)
		err := e.Start(addr)
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
	shutdownErr := e.Shutdown(shutdownCtx)
	database.DisconnectConnections()
	if shutdownErr != nil {
		log.Printf("Server shutdown error: %v", shutdownErr)
		return shutdownErr
	}

	log.Println("Server stopped.")
	return nil
}
