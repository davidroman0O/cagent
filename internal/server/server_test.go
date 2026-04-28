package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidroman0O/cagent/internal/agent"
	"github.com/davidroman0O/cagent/internal/compat"
	aruntime "github.com/davidroman0O/cagent/internal/runtime"
)

type testProvider struct {
	events []agent.AgentEvent
}

func (p testProvider) Name() string {
	return "test"
}

func (p testProvider) Capabilities() agent.ProviderCapabilities {
	return agent.ProviderCapabilities{Streaming: true, Resume: true}
}

func (p testProvider) Run(_ context.Context, _ agent.AgentRequest) (<-chan agent.AgentEvent, error) {
	ch := make(chan agent.AgentEvent, len(p.events))
	go func() {
		defer close(ch)
		for _, event := range p.events {
			ch <- event
		}
	}()
	return ch, nil
}

func TestChatCompletionsNonStreaming(t *testing.T) {
	srv := testServer(t, []agent.AgentEvent{
		{Type: agent.EventMessage, Message: "hello"},
		{Type: agent.EventDone, Final: "hello", Usage: &agent.Usage{InputTokens: 2, OutputTokens: 1}},
	}, "")

	body := `{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	choices := got["choices"].([]any)
	message := choices[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != "hello" {
		t.Fatalf("content = %#v", message["content"])
	}
}

func TestChatCompletionsStreaming(t *testing.T) {
	srv := testServer(t, []agent.AgentEvent{
		{Type: agent.EventMessage, Message: "he"},
		{Type: agent.EventMessage, Message: "hello"},
		{Type: agent.EventDone, Final: "hello"},
	}, "")

	body := `{"model":"test-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	if !strings.Contains(out, `"content":"he"`) || !strings.Contains(out, `"content":"llo"`) || !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("unexpected stream:\n%s", out)
	}
}

func TestAuth(t *testing.T) {
	srv := testServer(t, nil, "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestModelsAdvertiseContextOverrides(t *testing.T) {
	srv := testServerWithOptions(t, nil, "", Options{
		Defaults:                   compat.AgentDefaults{Model: "test-model"},
		ModelContextWindow:         1000000,
		ModelAutoCompactTokenLimit: 900000,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	model := got.Data[0]
	if model["context_window"] != float64(1000000) || model["model_auto_compact_token_limit"] != float64(900000) {
		t.Fatalf("model metadata = %#v", model)
	}
}

func TestRequestLoggingAndMetrics(t *testing.T) {
	var logs bytes.Buffer
	srv := testServerWithOptions(t, nil, "", Options{
		Defaults: compat.AgentDefaults{Model: "test-model"},
		Logger:   log.New(&logs, "", 0),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "req_test")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	logText := logs.String()
	if !strings.Contains(logText, "request started id=req_test method=GET path=/health") {
		t.Fatalf("missing start log:\n%s", logText)
	}
	if !strings.Contains(logText, "request completed id=req_test method=GET path=/health status=200") || !strings.Contains(logText, "duration_ms=") {
		t.Fatalf("missing completion log:\n%s", logText)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `cagent_http_requests_total{method="GET",path="/health",status="200"} 1`) {
		t.Fatalf("missing request metric:\n%s", body)
	}
	if !strings.Contains(body, `cagent_http_request_duration_seconds_sum{method="GET",path="/health",status="200"}`) {
		t.Fatalf("missing duration metric:\n%s", body)
	}
}

func TestResponsesNonStreaming(t *testing.T) {
	srv := testServer(t, []agent.AgentEvent{
		{Type: agent.EventMessage, Message: "result"},
		{Type: agent.EventDone, Final: "result"},
	}, "")
	body := []byte(`{"model":"test-model","input":"do work"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"result"`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}

func TestResponsesStreamingToolBridge(t *testing.T) {
	srv := testServer(t, []agent.AgentEvent{
		{Type: agent.EventMessage, Message: `{"cagent_tool_call":{"name":"start_mission_run","arguments":{"resumeWorkerSessionId":"abc"}}}`},
		{Type: agent.EventDone, Final: `{"cagent_tool_call":{"name":"start_mission_run","arguments":{"resumeWorkerSessionId":"abc"}}}`},
	}, "")
	body := []byte(`{
		"model":"test-model",
		"stream":true,
		"input":"do work",
		"tools":[{
			"type":"function",
			"name":"StartMissionRun",
			"description":"Start the mission runner",
			"parameters":{"type":"object","properties":{"resumeWorkerSessionId":{"type":"string"}}}
		}]
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	for _, want := range []string{
		"event: response.output_item.added",
		`"type":"function_call"`,
		`"name":"StartMissionRun"`,
		"event: response.function_call_arguments.delta",
		`\"resumeWorkerSessionId\":\"abc\"`,
		"data: [DONE]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stream missing %q:\n%s", want, out)
		}
	}
}

func testServer(t *testing.T, events []agent.AgentEvent, token string) *Server {
	t.Helper()
	return testServerWithOptions(t, events, token, Options{
		Defaults: compat.AgentDefaults{Model: "test-model"},
	})
}

func testServerWithOptions(t *testing.T, events []agent.AgentEvent, token string, opts Options) *Server {
	t.Helper()
	m, err := aruntime.NewManager([]agent.Provider{testProvider{events: events}}, aruntime.Options{
		DefaultProvider: "test",
		DataDir:         t.TempDir(),
		MaxConcurrent:   1,
		QueueLimit:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	opts.Manager = m
	opts.Token = token
	return New(Options{
		Manager:                    opts.Manager,
		Defaults:                   opts.Defaults,
		Token:                      opts.Token,
		Timeout:                    opts.Timeout,
		ModelContextWindow:         opts.ModelContextWindow,
		ModelAutoCompactTokenLimit: opts.ModelAutoCompactTokenLimit,
		Logger:                     opts.Logger,
	})
}
