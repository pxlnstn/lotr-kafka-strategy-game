package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"rotme/internal/api"
	"rotme/internal/config"
	"rotme/internal/engine"
)

func main() {
	brokers := splitEnv("KAFKA_BROKERS", "localhost:9092")
	srURL := getenv("SCHEMA_REGISTRY_URL", "http://localhost:8081")
	httpAddr := getenv("HTTP_ADDR", ":8080")
	turnSeconds := atoi(getenv("TURN_SECONDS", "60"))

	configDir := getenv("CONFIG_DIR", "")
	if configDir == "" {
		d, err := config.FindConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		configDir = d
	}
	cfg, err := config.Load(configDir)
	if err != nil {
		log.Fatal(err)
	}

	eng, err := newEngineWithRetry(cfg, brokers, srURL, turnSeconds)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := eng.Run(ctx); err != nil && ctx.Err() == nil {
			log.Fatalf("engine: %v", err)
		}
	}()

	srv := &http.Server{Addr: httpAddr, Handler: api.New(eng).Handler()}
	go func() {
		log.Printf("http listening on %s (brokers=%v sr=%s turn=%ds)", httpAddr, brokers, srURL, turnSeconds)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
}

// newEngineWithRetry waits for Kafka/Schema Registry to be reachable on startup.
func newEngineWithRetry(cfg *config.Config, brokers []string, srURL string, turnSeconds int) (*engine.Engine, error) {
	var lastErr error
	for i := 0; i < 30; i++ {
		eng, err := engine.New(cfg, brokers, srURL, turnSeconds)
		if err == nil {
			return eng, nil
		}
		lastErr = err
		log.Printf("waiting for kafka/schema-registry: %v", err)
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitEnv(k, def string) []string {
	return strings.Split(getenv(k, def), ",")
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	if n <= 0 {
		return 60
	}
	return n
}
