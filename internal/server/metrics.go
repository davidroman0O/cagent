package server

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HTTPMetrics struct {
	mu   sync.Mutex
	rows map[metricKey]metricValue
}

type metricKey struct {
	Method string
	Path   string
	Status int
}

type metricValue struct {
	Count       int64
	Bytes       int64
	DurationSum time.Duration
}

type metricRow struct {
	Key   metricKey
	Value metricValue
}

func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{rows: make(map[metricKey]metricValue)}
}

func (m *HTTPMetrics) Record(method, path string, status int, duration time.Duration, bytes int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := metricKey{Method: method, Path: path, Status: status}
	value := m.rows[key]
	value.Count++
	value.Bytes += bytes
	value.DurationSum += duration
	m.rows[key] = value
}

func (m *HTTPMetrics) Prometheus() string {
	rows := m.snapshot()
	var b strings.Builder
	b.WriteString("# HELP cagent_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE cagent_http_requests_total counter\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "cagent_http_requests_total%s %d\n", metricLabels(row.Key), row.Value.Count)
	}
	b.WriteString("# HELP cagent_http_response_bytes_total Total HTTP response bytes.\n")
	b.WriteString("# TYPE cagent_http_response_bytes_total counter\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "cagent_http_response_bytes_total%s %d\n", metricLabels(row.Key), row.Value.Bytes)
	}
	b.WriteString("# HELP cagent_http_request_duration_seconds_sum Total HTTP request duration in seconds.\n")
	b.WriteString("# TYPE cagent_http_request_duration_seconds_sum counter\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "cagent_http_request_duration_seconds_sum%s %.6f\n", metricLabels(row.Key), row.Value.DurationSum.Seconds())
	}
	b.WriteString("# HELP cagent_http_request_duration_seconds_count Total HTTP request duration observations.\n")
	b.WriteString("# TYPE cagent_http_request_duration_seconds_count counter\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "cagent_http_request_duration_seconds_count%s %d\n", metricLabels(row.Key), row.Value.Count)
	}
	return b.String()
}

func (m *HTTPMetrics) snapshot() []metricRow {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := make([]metricRow, 0, len(m.rows))
	for key, value := range m.rows {
		rows = append(rows, metricRow{Key: key, Value: value})
	}
	sort.Slice(rows, func(i, j int) bool {
		left, right := rows[i].Key, rows[j].Key
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Method != right.Method {
			return left.Method < right.Method
		}
		return left.Status < right.Status
	})
	return rows
}

func metricLabels(key metricKey) string {
	return fmt.Sprintf(
		`{method="%s",path="%s",status="%s"}`,
		escapeLabel(key.Method),
		escapeLabel(key.Path),
		strconv.Itoa(key.Status),
	)
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func metricPath(path string) string {
	if path == "/healthz" {
		return "/health"
	}
	if strings.HasPrefix(path, "/api/sessions/") {
		rest := strings.Trim(strings.TrimPrefix(path, "/api/sessions/"), "/")
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[1] == "turns" {
			return "/api/sessions/{id}/turns"
		}
		if len(parts) == 1 && parts[0] != "" {
			return "/api/sessions/{id}"
		}
	}
	return path
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) Flush() {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *statusRecorder) Bytes() int64 {
	return r.bytes
}

func requestID(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Request-ID")); value != "" {
		return value
	}
	return newID("req")
}

func remoteAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
