package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/poc-amp/backend/api/handlers"
	"github.com/poc-amp/backend/services"
	"github.com/poc-amp/backend/store"
)

func NewRouter(agentService *services.AgentService, suggestionService *services.SuggestionService, recoveryService *services.RecoveryService, db *store.Store) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://127.0.0.1:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	agentHandler := handlers.NewAgentHandler(agentService)
	compensationHandler := handlers.NewCompensationHandler(db, suggestionService, recoveryService)
	transactionHandler := handlers.NewTransactionHandler(db)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Agent management
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", agentHandler.ListAgents)
			r.Post("/", agentHandler.CreateAgent)
			r.Get("/{id}", agentHandler.GetAgent)
			r.Delete("/{id}", agentHandler.DeleteAgent)
			r.Post("/{id}/start", agentHandler.StartAgent)
			r.Post("/{id}/stop", agentHandler.StopAgent)
			r.Get("/{id}/logs", agentHandler.StreamLogs)

			// Tool registration and compensation mappings
			r.Post("/{id}/tools", compensationHandler.RegisterTools)
			r.Get("/{id}/compensation-mappings", compensationHandler.ListMappings)
			r.Get("/{id}/compensation-mappings/approved", compensationHandler.GetApprovedMappings)

			// Transaction logging
			r.Post("/{id}/transactions", compensationHandler.LogExecution)

			// Rollback
			r.Get("/{id}/sessions/{sessionId}/rollback-plan", compensationHandler.GetRollbackPlan)
			r.Post("/{id}/sessions/{sessionId}/rollback", compensationHandler.ExecuteRollback)

			// eBPF agent endpoints
			r.Post("/{id}/compensation-mappings/discover", transactionHandler.DiscoverCompensation)
			r.Post("/{id}/compensation-mappings/{mappingId}/approve", transactionHandler.ApproveCompensation)
		})

		// Compensation mapping management (cross-agent)
		r.Route("/compensation-mappings", func(r chi.Router) {
			r.Get("/{mappingId}", compensationHandler.GetMapping)
			r.Put("/{mappingId}", compensationHandler.UpdateMapping)
			r.Post("/{mappingId}/approve", compensationHandler.ApproveMapping)
			r.Post("/{mappingId}/reject", compensationHandler.RejectMapping)
		})
	})

	// Internal ingress for Envoy sidecar
	r.Route("/internal/envoy", func(r chi.Router) {
		r.Post("/transactions", HandleEnvoyTransaction(NewInterceptorServer(db)))
	})

	return r
}
