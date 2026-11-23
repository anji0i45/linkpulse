package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"linkpulse/internal/api"
	"linkpulse/internal/cache"
	"linkpulse/internal/queue"
	"linkpulse/internal/store"
)

func main() {
	dataPath := env("LINKPULSE_DATA", filepath.Join("data", "linkpulse.json"))
	st, err := store.NewFileStore(dataPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	linkCache := cache.NewMemory(5 * time.Minute)
	clickQueue := queue.New(st, 1024)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	clickQueue.Start(ctx)

	addr := env("ADDR", ":8080")
	baseURL := env("PUBLIC_BASE_URL", "")
	handler := api.NewServer(st, linkCache, clickQueue, baseURL)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("LinkPulse running at http://localhost%s", addr)
		log.Printf("data file: %s", dataPath)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	case <-ctx.Done():
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	clickQueue.Wait()
	log.Println("LinkPulse stopped")
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
