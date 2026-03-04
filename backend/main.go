package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/poc-amp/backend/api"
	"github.com/poc-amp/backend/config"
	pb "github.com/poc-amp/backend/proto/v1"
	"github.com/poc-amp/backend/services"
	"github.com/poc-amp/backend/store"
	"google.golang.org/grpc"
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
		log.Printf("Starting HTTP server on port %s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Start the gRPC Interceptor Server
	grpcLis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen for gRPC on :50051: %v", err)
	}

	grpcServer := grpc.NewServer()
	interceptorSvc := api.NewInterceptorServer(db)
	pb.RegisterTransactionServiceServer(grpcServer, interceptorSvc)

	go func() {
		log.Printf("Starting gRPC interceptor server on :50051")
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
}
