package runtime

import (
	"context"
	"testing"

	"github.com/davidroman0O/cagent/internal/agent"
)

type fakeProvider struct {
	events []agent.AgentEvent
}

func (p fakeProvider) Name() string {
	return "fake"
}

func (p fakeProvider) Capabilities() agent.ProviderCapabilities {
	return agent.ProviderCapabilities{Streaming: true, Resume: true}
}

func (p fakeProvider) Run(_ context.Context, _ agent.AgentRequest) (<-chan agent.AgentEvent, error) {
	ch := make(chan agent.AgentEvent, len(p.events))
	go func() {
		defer close(ch)
		for _, ev := range p.events {
			ch <- ev
		}
	}()
	return ch, nil
}

func TestManagerPersistsProviderSessionID(t *testing.T) {
	provider := fakeProvider{events: []agent.AgentEvent{
		{Type: agent.EventStarted, ProviderSessionID: "provider_1"},
		{Type: agent.EventDone, Final: "done"},
	}}
	m, err := NewManager([]agent.Provider{provider}, Options{
		DefaultProvider: "fake",
		DataDir:         t.TempDir(),
		MaxConcurrent:   1,
		QueueLimit:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := m.CreateSession(agent.AgentRequest{Provider: "fake", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	events, err := m.Run(context.Background(), agent.AgentRequest{SessionID: session.ID, Provider: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	updated, ok := m.GetSession(session.ID)
	if !ok {
		t.Fatal("session missing")
	}
	if updated.ProviderSessionID != "provider_1" {
		t.Fatalf("provider session id = %q", updated.ProviderSessionID)
	}
	if updated.Status != "ready" {
		t.Fatalf("status = %q", updated.Status)
	}
}
