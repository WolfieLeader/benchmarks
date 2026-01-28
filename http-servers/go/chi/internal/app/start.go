package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func (app *App) Start() {
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.env.HOST, app.env.PORT),
		Handler:      app.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Printf("Chi Server development: http://%s\n\n", server.Addr)

		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	cancel()
	fmt.Println("\nShutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)

	if err := server.Shutdown(shutdownCtx); err != nil {
		shutdownCancel()
		_ = server.Close()
		log.Fatalf("Server shutdown error: %v", err)
	}
	shutdownCancel()

	fmt.Println("Server stopped.")
}
