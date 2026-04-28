package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
)

var (
	ErrUnknownProvider = errors.New("unknown provider")
	ErrUnknownSession  = errors.New("unknown session")
	ErrSessionBusy     = errors.New("session already has a running turn")
	ErrQueueFull       = errors.New("runtime queue is full")
)

type Session struct {
	ID                string    `json:"id"`
	Provider          string    `json:"provider"`
	ProviderSessionID string    `json:"provider_session_id,omitempty"`
	Model             string    `json:"model,omitempty"`
	CWD               string    `json:"cwd,omitempty"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Manager struct {
	providers       map[string]agent.Provider
	defaultProvider string
	dataDir         string

	sem        chan struct{}
	queueLimit int

	mu       sync.Mutex
	queued   int
	sessions map[string]*Session
	running  map[string]bool
}

type Options struct {
	DefaultProvider string
	DataDir         string
	MaxConcurrent   int
	QueueLimit      int
}

func NewManager(providers []agent.Provider, opts Options) (*Manager, error) {
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = 1
	}
	if opts.QueueLimit <= 0 {
		opts.QueueLimit = opts.MaxConcurrent * 4
	}
	m := &Manager{
		providers:       make(map[string]agent.Provider),
		defaultProvider: opts.DefaultProvider,
		dataDir:         opts.DataDir,
		sem:             make(chan struct{}, opts.MaxConcurrent),
		queueLimit:      opts.QueueLimit,
		sessions:        make(map[string]*Session),
		running:         make(map[string]bool),
	}
	for _, provider := range providers {
		m.providers[provider.Name()] = provider
		if m.defaultProvider == "" {
			m.defaultProvider = provider.Name()
		}
	}
	if len(m.providers) == 0 {
		return nil, errors.New("at least one provider is required")
	}
	_ = m.loadSessions()
	return m, nil
}

func (m *Manager) Providers() map[string]agent.ProviderCapabilities {
	out := make(map[string]agent.ProviderCapabilities, len(m.providers))
	for name, provider := range m.providers {
		out[name] = provider.Capabilities()
	}
	return out
}

func (m *Manager) CreateSession(req agent.AgentRequest) (*Session, error) {
	providerName := m.providerName(req.Provider)
	if _, ok := m.providers[providerName]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, providerName)
	}
	now := time.Now().UTC()
	session := &Session{
		ID:        newID("sess"),
		Provider:  providerName,
		Model:     req.Model,
		CWD:       req.CWD,
		Status:    "ready",
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()
	_ = m.saveSessions()
	return session, nil
}

func (m *Manager) GetSession(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	copy := *session
	return &copy, true
}

func (m *Manager) Run(ctx context.Context, req agent.AgentRequest) (<-chan agent.AgentEvent, error) {
	providerName := m.providerName(req.Provider)
	provider, ok := m.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, providerName)
	}

	if req.SessionID != "" {
		session, err := m.bindSession(&req, providerName)
		if err != nil {
			return nil, err
		}
		req.ProviderSessionID = session.ProviderSessionID
		if req.Model == "" {
			req.Model = session.Model
		}
		if req.CWD == "" {
			req.CWD = session.CWD
		}
	}

	if err := m.acquire(ctx); err != nil {
		if req.SessionID != "" {
			m.clearRunning(req.SessionID)
		}
		return nil, err
	}

	providerEvents, err := provider.Run(ctx, req)
	if err != nil {
		m.release()
		if req.SessionID != "" {
			m.clearRunning(req.SessionID)
		}
		return nil, err
	}

	out := make(chan agent.AgentEvent, 32)
	go func() {
		defer close(out)
		defer m.release()
		if req.SessionID != "" {
			defer m.clearRunning(req.SessionID)
			m.setSessionStatus(req.SessionID, "running")
		}
		for event := range providerEvents {
			event.SessionID = firstNonEmpty(event.SessionID, req.SessionID)
			if event.ProviderSessionID != "" && req.SessionID != "" {
				m.setProviderSessionID(req.SessionID, event.ProviderSessionID)
			}
			if event.Type == agent.EventDone && req.SessionID != "" {
				m.setSessionStatus(req.SessionID, "ready")
			}
			if event.Type == agent.EventError && req.SessionID != "" {
				m.setSessionStatus(req.SessionID, "failed")
			}
			_ = m.appendEvent(event)
			out <- event
		}
	}()
	return out, nil
}

func (m *Manager) bindSession(req *agent.AgentRequest, providerName string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[req.SessionID]
	if !ok {
		return nil, ErrUnknownSession
	}
	if session.Provider != providerName {
		return nil, fmt.Errorf("session provider mismatch: %s != %s", session.Provider, providerName)
	}
	if m.running[req.SessionID] {
		return nil, ErrSessionBusy
	}
	m.running[req.SessionID] = true
	return session, nil
}

func (m *Manager) clearRunning(sessionID string) {
	m.mu.Lock()
	delete(m.running, sessionID)
	m.mu.Unlock()
}

func (m *Manager) setProviderSessionID(sessionID, providerSessionID string) {
	m.mu.Lock()
	if session := m.sessions[sessionID]; session != nil {
		session.ProviderSessionID = providerSessionID
		session.UpdatedAt = time.Now().UTC()
	}
	m.mu.Unlock()
	_ = m.saveSessions()
}

func (m *Manager) setSessionStatus(sessionID, status string) {
	m.mu.Lock()
	if session := m.sessions[sessionID]; session != nil {
		session.Status = status
		session.UpdatedAt = time.Now().UTC()
	}
	m.mu.Unlock()
	_ = m.saveSessions()
}

func (m *Manager) providerName(name string) string {
	if name != "" {
		return name
	}
	return m.defaultProvider
}

func (m *Manager) acquire(ctx context.Context) error {
	m.mu.Lock()
	if m.queued >= m.queueLimit {
		m.mu.Unlock()
		return ErrQueueFull
	}
	m.queued++
	m.mu.Unlock()

	select {
	case m.sem <- struct{}{}:
		m.mu.Lock()
		m.queued--
		m.mu.Unlock()
		return nil
	case <-ctx.Done():
		m.mu.Lock()
		m.queued--
		m.mu.Unlock()
		return ctx.Err()
	}
}

func (m *Manager) release() {
	select {
	case <-m.sem:
	default:
	}
}

func (m *Manager) sessionsPath() string {
	if m.dataDir == "" {
		return ""
	}
	return filepath.Join(m.dataDir, "sessions.json")
}

func (m *Manager) eventsPath(sessionID string) string {
	if m.dataDir == "" || sessionID == "" {
		return ""
	}
	return filepath.Join(m.dataDir, "events", sessionID+".jsonl")
}

func (m *Manager) saveSessions() error {
	path := m.sessionsPath()
	if path == "" {
		return nil
	}
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		copy := *session
		sessions = append(sessions, &copy)
	}
	m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (m *Manager) loadSessions() error {
	path := m.sessionsPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sessions []*Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return err
	}
	m.mu.Lock()
	for _, session := range sessions {
		copy := *session
		if copy.Status == "running" || copy.Status == "starting" {
			copy.Status = "failed"
		}
		m.sessions[copy.ID] = &copy
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) appendEvent(event agent.AgentEvent) error {
	path := m.eventsPath(event.SessionID)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
