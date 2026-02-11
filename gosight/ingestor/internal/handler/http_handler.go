package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/gosight/gosight/ingestor/internal/enricher"
	"github.com/gosight/gosight/ingestor/internal/producer"
	"github.com/gosight/gosight/ingestor/internal/validation"
)

type HTTPHandler struct {
	producer  *producer.KafkaProducer
	validator *validation.Validator
	enricher  *enricher.Enricher
}

func NewHTTPHandler(p *producer.KafkaProducer, v *validation.Validator, e *enricher.Enricher) *HTTPHandler {
	return &HTTPHandler{
		producer:  p,
		validator: v,
		enricher:  e,
	}
}

type EventBatchRequest struct {
	ProjectKey string                   `json:"project_key"`
	SessionID  string                   `json:"session_id"`
	UserID     string                   `json:"user_id"`
	Events     []map[string]interface{} `json:"events"`
}

type EventResponse struct {
	Success       bool     `json:"success"`
	AcceptedCount int      `json:"accepted_count"`
	RejectedCount int      `json:"rejected_count"`
	Errors        []string `json:"errors,omitempty"`
}

func (h *HTTPHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse request
	var req EventBatchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate API key
	projectID, err := h.validator.ValidateAPIKey(r.Context(), req.ProjectKey)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(EventResponse{
			Success: false,
			Errors:  []string{"Invalid API key"},
		})
		return
	}

	// Rate limiting
	if !h.validator.CheckRateLimit(projectID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(EventResponse{
			Success: false,
			Errors:  []string{"Rate limit exceeded"},
		})
		return
	}

	// Get client IP for enrichment
	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Forwarded-For")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	// Get User-Agent
	userAgent := r.Header.Get("User-Agent")

	// Process events
	accepted := 0
	rejected := 0
	var errors []string

	for _, event := range req.Events {
		// Add metadata
		event["project_id"] = projectID
		event["session_id"] = req.SessionID
		event["user_id"] = req.UserID
		if event["event_id"] == nil {
			event["event_id"] = uuid.New().String()
		}

		// Enrich event
		enrichedEvent := h.enricher.Enrich(event, userAgent, clientIP)

		// Produce to Kafka
		err := h.producer.ProduceEvent(r.Context(), projectID, enrichedEvent)
		if err != nil {
			rejected++
			errors = append(errors, err.Error())
			continue
		}
		accepted++
	}

	// Response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(EventResponse{
		Success:       rejected == 0,
		AcceptedCount: accepted,
		RejectedCount: rejected,
		Errors:        errors,
	})
}

type ReplayChunkRequest struct {
	ProjectKey     string `json:"project_key"`
	SessionID      string `json:"session_id"`
	ChunkIndex     int    `json:"chunk_index"`
	TimestampStart int64  `json:"timestamp_start"`
	TimestampEnd   int64  `json:"timestamp_end"`
	Data           string `json:"data"` // Base64 encoded compressed rrweb events
	HasFullSnapshot bool  `json:"has_full_snapshot"`
}

func (h *HTTPHandler) HandleReplay(w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse request
	var req ReplayChunkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate API key
	projectID, err := h.validator.ValidateAPIKey(r.Context(), req.ProjectKey)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Invalid API key",
		})
		return
	}

	// Rate limiting
	if !h.validator.CheckRateLimit(projectID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Rate limit exceeded",
		})
		return
	}

	// Create chunk message
	chunk := map[string]interface{}{
		"project_id":        projectID,
		"session_id":        req.SessionID,
		"chunk_index":       req.ChunkIndex,
		"timestamp_start":   req.TimestampStart,
		"timestamp_end":     req.TimestampEnd,
		"data":              req.Data,
		"has_full_snapshot": req.HasFullSnapshot,
	}

	// Produce to Kafka replay topic
	err = h.producer.ProduceReplayChunk(r.Context(), chunk)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Chunk received",
	})
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Project-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
