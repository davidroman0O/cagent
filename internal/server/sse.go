package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
)

func writeEventStream(w http.ResponseWriter, events <-chan agent.AgentEvent) {
	startSSE(w)
	flusher, _ := w.(http.Flusher)
	for event := range events {
		writeSSE(w, "agent."+string(event.Type), event)
		if flusher != nil {
			flusher.Flush()
		}
	}
	writeRawSSE(w, "done", "[DONE]")
}

func writeChatSSE(w http.ResponseWriter, id, model string, events <-chan agent.AgentEvent) {
	startSSE(w)
	flusher, _ := w.(http.Flusher)
	created := time.Now().Unix()
	writeSSE(w, "", chatChunk{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: []chatStreamChoice{{Index: 0, Delta: map[string]string{"role": "assistant"}}},
	})
	flush(flusher)

	last := ""
	for event := range events {
		switch event.Type {
		case agent.EventDelta:
			if event.Delta != "" {
				writeChatDelta(w, id, model, created, event.Delta)
				flush(flusher)
				last += event.Delta
			}
		case agent.EventMessage:
			delta := suffixDelta(last, event.Message)
			if delta != "" {
				writeChatDelta(w, id, model, created, delta)
				flush(flusher)
				last = event.Message
			}
		case agent.EventDone:
			if event.Final != "" {
				delta := suffixDelta(last, event.Final)
				if delta != "" {
					writeChatDelta(w, id, model, created, delta)
					flush(flusher)
				}
			}
		case agent.EventError:
			writeSSE(w, "error", map[string]any{"message": event.Err})
			flush(flusher)
		}
	}
	writeSSE(w, "", chatChunk{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: []chatStreamChoice{{Index: 0, Delta: map[string]string{}, FinishReason: "stop"}},
	})
	writeRawSSE(w, "", "[DONE]")
	flush(flusher)
}

func writeResponsesSSE(w http.ResponseWriter, id, model string, events <-chan agent.AgentEvent) {
	startSSE(w)
	flusher, _ := w.(http.Flusher)
	created := time.Now().Unix()
	writeSSE(w, "response.created", map[string]any{
		"type":     "response.created",
		"response": map[string]any{"id": id, "object": "response", "created_at": created, "model": model, "status": "in_progress"},
	})
	writeSSE(w, "response.in_progress", map[string]any{
		"type":     "response.in_progress",
		"response": map[string]any{"id": id, "status": "in_progress"},
	})
	flush(flusher)

	last := ""
	itemStarted := false
	for event := range events {
		switch event.Type {
		case agent.EventDelta:
			if event.Delta != "" {
				ensureResponseItem(w, id, &itemStarted)
				writeResponsesDelta(w, id, event.Delta)
				last += event.Delta
				flush(flusher)
			}
		case agent.EventMessage:
			delta := suffixDelta(last, event.Message)
			if delta != "" {
				ensureResponseItem(w, id, &itemStarted)
				writeResponsesDelta(w, id, delta)
				last = event.Message
				flush(flusher)
			}
		case agent.EventDone:
			if event.Final != "" {
				delta := suffixDelta(last, event.Final)
				if delta != "" {
					ensureResponseItem(w, id, &itemStarted)
					writeResponsesDelta(w, id, delta)
				}
				last = event.Final
			}
		case agent.EventError:
			writeSSE(w, "response.failed", map[string]any{"type": "response.failed", "error": map[string]any{"message": event.Err}})
			flush(flusher)
		}
	}
	if itemStarted {
		writeSSE(w, "response.output_text.done", map[string]any{"type": "response.output_text.done", "item_id": "msg_" + id, "output_index": 0, "content_index": 0, "text": last})
		writeSSE(w, "response.content_part.done", map[string]any{"type": "response.content_part.done", "item_id": "msg_" + id, "output_index": 0, "content_index": 0})
		writeSSE(w, "response.output_item.done", map[string]any{"type": "response.output_item.done", "item": map[string]any{"id": "msg_" + id, "type": "message", "role": "assistant"}})
	}
	writeSSE(w, "response.completed", map[string]any{
		"type":     "response.completed",
		"response": map[string]any{"id": id, "status": "completed", "output_text": last},
	})
	writeRawSSE(w, "", "[DONE]")
	flush(flusher)
}

type chatChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []chatStreamChoice `json:"choices"`
}

type chatStreamChoice struct {
	Index        int               `json:"index"`
	Delta        map[string]string `json:"delta"`
	FinishReason string            `json:"finish_reason,omitempty"`
}

func writeChatDelta(w http.ResponseWriter, id, model string, created int64, delta string) {
	writeSSE(w, "", chatChunk{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: []chatStreamChoice{{Index: 0, Delta: map[string]string{"content": delta}}},
	})
}

func ensureResponseItem(w http.ResponseWriter, id string, started *bool) {
	if *started {
		return
	}
	*started = true
	writeSSE(w, "response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item":         map[string]any{"id": "msg_" + id, "type": "message", "role": "assistant", "content": []any{}},
	})
	writeSSE(w, "response.content_part.added", map[string]any{
		"type":          "response.content_part.added",
		"item_id":       "msg_" + id,
		"output_index":  0,
		"content_index": 0,
		"part":          map[string]any{"type": "output_text", "text": ""},
	})
}

func writeResponsesDelta(w http.ResponseWriter, id, delta string) {
	writeSSE(w, "response.output_text.delta", map[string]any{
		"type":          "response.output_text.delta",
		"item_id":       "msg_" + id,
		"output_index":  0,
		"content_index": 0,
		"delta":         delta,
	})
}

func startSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

func writeSSE(w http.ResponseWriter, event string, value any) {
	data, _ := json.Marshal(value)
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func writeRawSSE(w http.ResponseWriter, event, data string) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func flush(flusher http.Flusher) {
	if flusher != nil {
		flusher.Flush()
	}
}

func suffixDelta(previous, current string) string {
	if current == "" {
		return ""
	}
	if previous == "" {
		return current
	}
	if len(current) >= len(previous) && current[:len(previous)] == previous {
		return current[len(previous):]
	}
	return current
}
