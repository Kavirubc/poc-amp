package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/poc-amp/backend/models"
	pb "github.com/poc-amp/backend/proto/v1"
	"github.com/poc-amp/backend/store"
)

type InterceptorServer struct {
	pb.UnimplementedTransactionServiceServer
	db *store.Store
}

func NewInterceptorServer(db *store.Store) *InterceptorServer {
	return &InterceptorServer{
		db: db,
	}
}

func (s *InterceptorServer) StreamTransactions(stream pb.TransactionService_StreamTransactionsServer) error {
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.TransactionResponse{
				Success: true,
				Message: "Stream closed normally",
			})
		}
		if err != nil {
			log.Printf("Error receiving from Envoy stream: %v", err)
			return err
		}

		if err := s.ProcessSingleEvent(event); err != nil {
			log.Printf("Error processing event: %v", err)
		}
	}
}

// ProcessSingleEvent processes a single transaction event received via gRPC or HTTP Bridge
func (s *InterceptorServer) ProcessSingleEvent(event *pb.TransactionEvent) error {
	log.Printf("Received Intercepted Transaction from Envoy [Agent: %s]: %s %s", event.AgentId, event.Method, event.Url)

	// Ensure the agent row exists so the FK constraint is satisfied.
	// For Envoy-intercepted traffic, we upsert a synthetic sentinel agent.
	if err := s.ensureAgentExists(event.AgentId); err != nil {
		log.Printf("Failed to upsert sentinel agent: %v", err)
		return err
	}

	// Parse the JSON bodies into raw messages for storage
	var requestData, responseData json.RawMessage

	if len(event.RequestBody) > 0 {
		requestData = json.RawMessage(event.RequestBody)
	} else {
		requestData = json.RawMessage("{}")
	}
	if len(event.ResponseBody) > 0 {
		responseData = json.RawMessage(event.ResponseBody)
	} else {
		responseData = json.RawMessage("{}")
	}

	// Look up the tool execution mapping (heuristic mapping from URL/Method to ToolName)
	toolName := s.extractToolName(event.Url, event.Method)

	now := time.Now()
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return fmt.Errorf("failed to generate transaction ID: %w", err)
	}
	txID := hex.EncodeToString(idBytes)

	tx := &models.TransactionLog{
		ID:           txID,
		AgentID:      event.AgentId,
		SessionID:    event.SessionId,
		ToolName:     toolName,
		InputParams:  requestData,
		OutputResult: responseData,
		Status:       "executed",
		ExecutedAt:   now,
		CreatedAt:    now,
	}

	if event.ResponseStatus >= 400 {
		tx.Status = "error"
	}

	if err := s.db.CreateTransactionLog(tx); err != nil {
		log.Printf("Failed to save intercepted transaction: %v", err)
		return err
	}

	log.Printf("Successfully logged intercepted transaction %s", tx.ID)
	return nil
}

// ensureAgentExists upserts a sentinel agent row for Envoy-sourced transactions.
func (s *InterceptorServer) ensureAgentExists(agentID string) error {
	return s.db.EnsureEnvoyAgent(agentID)
}

// extractToolName tries to guess the semantic tool name from the raw intercepted HTTP URL
func (s *InterceptorServer) extractToolName(url string, method string) string {
	// A naive mapping for demonstration. In a production system, this could look up registered tools
	// or Envoy could pass the tool name in an explicit HTTP header (e.g., x-amp-tool-name) injected by the SDK.
	// Since we are doing a transparent proxy, we do a basic path inference.

	// Default to just storing the URL if we can't infer it
	return "intercept_" + method + "_" + url
}

// REST HTTP Ingress for Envoy Lua
type envoyPayload struct {
	AgentID        string `json:"agent_id"`
	SessionID      string `json:"session_id"`
	Method         string `json:"method"`
	URL            string `json:"url"`
	RequestBody    string `json:"request_body"`
	ResponseStatus int32  `json:"response_status"`
	ResponseBody   string `json:"response_body"`
	SourceIP       string `json:"source_ip,omitempty"` // Container IP from iptables transparent proxy
}

func HandleEnvoyTransaction(interceptorSvc *InterceptorServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload envoyPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		event := &pb.TransactionEvent{
			AgentId:        payload.AgentID,
			SessionId:      payload.SessionID,
			Method:         payload.Method,
			Url:            payload.URL,
			RequestBody:    []byte(payload.RequestBody),
			ResponseStatus: payload.ResponseStatus,
			ResponseBody:   []byte(payload.ResponseBody),
		}

		err := interceptorSvc.ProcessSingleEvent(event)
		if err != nil {
			http.Error(w, "Failed to process transaction", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}
