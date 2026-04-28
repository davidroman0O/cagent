package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
	"github.com/davidroman0O/cagent/internal/compat"
	aruntime "github.com/davidroman0O/cagent/internal/runtime"
)

type Server struct {
	manager                    *aruntime.Manager
	defaults                   compat.AgentDefaults
	token                      string
	timeout                    time.Duration
	modelContextWindow         int
	modelAutoCompactTokenLimit int
	logger                     *log.Logger
	metrics                    *HTTPMetrics
}

type Options struct {
	Manager                    *aruntime.Manager
	Defaults                   compat.AgentDefaults
	Token                      string
	Timeout                    time.Duration
	ModelContextWindow         int
	ModelAutoCompactTokenLimit int
	Logger                     *log.Logger
}

func New(opts Options) *Server {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	return &Server{
		manager:                    opts.Manager,
		defaults:                   opts.Defaults,
		token:                      opts.Token,
		timeout:                    opts.Timeout,
		modelContextWindow:         opts.ModelContextWindow,
		modelAutoCompactTokenLimit: opts.ModelAutoCompactTokenLimit,
		logger:                     opts.Logger,
		metrics:                    NewHTTPMetrics(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/metrics", s.withAuth(s.handleMetrics))
	mux.HandleFunc("/v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("/models", s.withAuth(s.handleModels))
	mux.HandleFunc("/v1/chat/completions", s.withAuth(s.handleChatCompletions))
	mux.HandleFunc("/v1/responses", s.withAuth(s.handleResponses))
	mux.HandleFunc("/api/providers", s.withAuth(s.handleProviders))
	mux.HandleFunc("/api/sessions", s.withAuth(s.handleSessions))
	mux.HandleFunc("/api/sessions/", s.withAuth(s.handleSessionPath))
	return s.withRequestLogging(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "cagent",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	model := s.defaults.Model
	if model == "" {
		model = "codex-default"
	}
	modelData := map[string]any{
		"id":       model,
		"object":   "model",
		"created":  time.Now().Unix(),
		"owned_by": "cagent",
	}
	if s.modelContextWindow > 0 {
		modelData["context_window"] = s.modelContextWindow
		modelData["max_context_window"] = s.modelContextWindow
		modelData["model_context_window"] = s.modelContextWindow
	}
	if s.modelAutoCompactTokenLimit > 0 {
		modelData["model_auto_compact_token_limit"] = s.modelAutoCompactTokenLimit
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   []map[string]any{modelData},
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = io.WriteString(w, s.metrics.Prometheus())
}

func (s *Server) handleProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": s.manager.Providers()})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req agent.AgentRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := s.manager.CreateSession(req)
	if err != nil {
		writeError(w, statusForRuntimeError(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleSessionPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		session, ok := s.manager.GetSession(parts[0])
		if !ok {
			writeError(w, http.StatusNotFound, aruntime.ErrUnknownSession)
			return
		}
		writeJSON(w, http.StatusOK, session)
		return
	}
	if len(parts) == 2 && parts[1] == "turns" && r.Method == http.MethodPost {
		s.handleSessionTurn(w, r, parts[0])
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleSessionTurn(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req agent.AgentRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.SessionID = sessionID
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	events, err := s.manager.Run(ctx, req)
	if err != nil {
		writeError(w, statusForRuntimeError(err), err)
		return
	}
	if r.URL.Query().Get("stream") == "true" {
		writeEventStream(w, events)
		return
	}
	text, usage, err := collect(events)
	if err != nil {
		writeError(w, statusForRuntimeError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"text":       text,
		"usage":      usage,
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req compat.ChatCompletionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	agentReq := compat.ChatToAgent(req, s.defaults)
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	events, err := s.manager.Run(ctx, agentReq)
	if err != nil {
		writeError(w, statusForRuntimeError(err), err)
		return
	}
	id := newID("chatcmpl")
	if req.Stream {
		writeChatSSE(w, id, agentReq.Model, events)
		return
	}
	text, usage, err := collect(events)
	if err != nil {
		writeError(w, statusForRuntimeError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, compat.NewChatCompletion(id, agentReq.Model, text, usage))
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req compat.ResponsesRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	agentReq := compat.ResponsesToAgent(req, s.defaults)
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	events, err := s.manager.Run(ctx, agentReq)
	if err != nil {
		writeError(w, statusForRuntimeError(err), err)
		return
	}
	id := newID("resp")
	if req.Stream {
		writeResponsesSSE(w, id, agentReq.Model, events)
		return
	}
	text, usage, err := collect(events)
	if err != nil {
		writeError(w, statusForRuntimeError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, compat.NewResponsesObject(id, agentReq.Model, text, usage))
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			next(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
		if token == "" {
			token = r.Header.Get("x-api-key")
		}
		if token != s.token {
			writeError(w, http.StatusUnauthorized, errors.New("missing or invalid API token"))
			return
		}
		next(w, r)
	}
}

func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestID(r)
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		s.logger.Printf("request started id=%s method=%s path=%s remote=%s", requestID, r.Method, r.URL.Path, remoteAddr(r))
		defer func() {
			status := rec.Status()
			duration := time.Since(start)
			s.metrics.Record(r.Method, metricPath(r.URL.Path), status, duration, rec.Bytes())
			s.logger.Printf(
				"request completed id=%s method=%s path=%s status=%d bytes=%d duration_ms=%d",
				requestID,
				r.Method,
				r.URL.Path,
				status,
				rec.Bytes(),
				duration.Milliseconds(),
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

func collect(events <-chan agent.AgentEvent) (string, *agent.Usage, error) {
	var text string
	var usage *agent.Usage
	for event := range events {
		switch event.Type {
		case agent.EventMessage:
			if event.Message != "" {
				text = event.Message
			}
		case agent.EventDelta:
			text += event.Delta
		case agent.EventDone:
			if event.Final != "" {
				text = event.Final
			}
			if event.Usage != nil {
				usage = event.Usage
			}
		case agent.EventUsage:
			if event.Usage != nil {
				usage = event.Usage
			}
		case agent.EventError:
			return text, usage, errors.New(event.Err)
		}
	}
	return text, usage, nil
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16*1024*1024))
	decoder.UseNumber()
	return decoder.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": err.Error(),
			"type":    "cagent_error",
		},
	})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func statusForRuntimeError(err error) int {
	switch {
	case errors.Is(err, aruntime.ErrUnknownSession):
		return http.StatusNotFound
	case errors.Is(err, aruntime.ErrSessionBusy), errors.Is(err, aruntime.ErrQueueFull):
		return http.StatusTooManyRequests
	case errors.Is(err, aruntime.ErrUnknownProvider):
		return http.StatusBadRequest
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return http.StatusGatewayTimeout
	default:
		return http.StatusBadGateway
	}
}

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
