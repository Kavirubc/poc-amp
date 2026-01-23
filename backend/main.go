package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/poc-amp/backend/api"
	"github.com/poc-amp/backend/config"
	"github.com/poc-amp/backend/services"
	"github.com/poc-amp/backend/store"
)

func main() {
	godotenv.Load()

	cfg := config.Load()

	log.Printf("Connecting to database...")
	var db *store.Store
	var err error
	for i := 0; i < 30; i++ {
		db, err = store.New(cfg.DatabaseURL)
		if err == nil {
			break
		}
		log.Printf("Database not ready, retrying in 2s... (%d/30)", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Printf("Connected to database")

	gitService := services.NewGitService(cfg.WorkspacePath)

	dockerService, err := services.NewDockerService(cfg.NetworkName)
	if err != nil {
		log.Fatalf("Failed to initialize docker service: %v", err)
	}
	defer dockerService.Close()
	log.Printf("Docker service initialized")

	agentService := services.NewAgentService(
		db,
		gitService,
		dockerService,
		cfg.PortRangeStart,
		cfg.PortRangeEnd,
	)

	// Initialize compensation services
	suggestionService := services.NewSuggestionService(db)
	recoveryService := services.NewRecoveryService(db)
	log.Printf("Compensation services initialized")

	router := api.NewRouter(agentService, suggestionService, recoveryService, db)

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Starting server on port %s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
}
