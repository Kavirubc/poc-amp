package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/poc-amp/backend/api/handlers"
	"github.com/poc-amp/backend/services"
)

func NewRouter(agentService *services.AgentService) http.Handler {
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

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", agentHandler.ListAgents)
			r.Post("/", agentHandler.CreateAgent)
			r.Get("/{id}", agentHandler.GetAgent)
			r.Delete("/{id}", agentHandler.DeleteAgent)
			r.Post("/{id}/start", agentHandler.StartAgent)
			r.Post("/{id}/stop", agentHandler.StopAgent)
			r.Get("/{id}/logs", agentHandler.StreamLogs)
		})
	})

	return r
}
