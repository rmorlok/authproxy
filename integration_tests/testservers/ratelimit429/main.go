package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// Server is a simple HTTP server that can be configured to return 429 responses.
// It tracks request counts so tests can verify which requests reached the upstream.
type Server struct {
	mu              sync.Mutex
	requestCount    atomic.Int64
	return429       bool
	retryAfterValue string // Value for Retry-After header; empty means no header
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/configure":
		s.handleConfigure(w, r)
	case "/stats":
		s.handleStats(w, r)
	case "/reset":
		s.handleReset(w, r)
	default:
		s.handleRequest(w, r)
	}
}

type configureRequest struct {
	Return429       bool   `json:"return_429"`
	RetryAfterValue string `json:"retry_after_value"`
}

func (s *Server) handleConfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req configureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.return429 = req.Return429
	s.retryAfterValue = req.RetryAfterValue
	s.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "configured"})
}

type statsResponse struct {
	RequestCount int64 `json:"request_count"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statsResponse{
		RequestCount: s.requestCount.Load(),
	})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.return429 = false
	s.retryAfterValue = ""
	s.mu.Unlock()
	s.requestCount.Store(0)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.requestCount.Add(1)

	s.mu.Lock()
	shouldReturn429 := s.return429
	retryAfter := s.retryAfterValue
	s.mu.Unlock()

	if shouldReturn429 {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limited"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"path":    r.URL.Path,
		"method":  r.Method,
		"request": s.requestCount.Load(),
	})
}

func main() {
	port := 9999
	if p := os.Getenv("PORT"); p != "" {
		var err error
		port, err = strconv.Atoi(p)
		if err != nil {
			log.Fatalf("invalid PORT: %v", err)
		}
	}

	s := &Server{}
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting test server on %s", addr)
	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}
