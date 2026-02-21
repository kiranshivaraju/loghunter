package loki

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- helpers ---

func lokiServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func newTestClient(t *testing.T, baseURL string) *HTTPClient {
	t.Helper()
	return NewHTTPClient(baseURL, "", "", "", 5*time.Second)
}

// --- QueryRange tests ---

func TestQueryRange_ValidResponse(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Verify query params
		q := r.URL.Query()
		if q.Get("query") != `{service="payments-api"}` {
			t.Errorf("unexpected query: %s", q.Get("query"))
		}
		if q.Get("limit") != "100" {
			t.Errorf("unexpected limit: %s", q.Get("limit"))
		}

		resp := lokiQueryResponse{
			Data: lokiData{
				ResultType: "streams",
				Result: []lokiStream{
					{
						Stream: map[string]string{
							"service": "payments-api",
							"level":   "error",
						},
						Values: [][2]string{
							{"1708128000000000000", "connection refused to database"},
							{"1708128060000000000", "retry attempt 1 failed"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	start := time.Date(2024, 2, 17, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 2, 17, 1, 0, 0, 0, time.UTC)

	lines, err := c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="payments-api"}`,
		Start: start,
		End:   end,
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Check first line
	if lines[0].Message != "connection refused to database" {
		t.Errorf("unexpected message: %s", lines[0].Message)
	}
	if lines[0].Labels["service"] != "payments-api" {
		t.Errorf("unexpected service label: %s", lines[0].Labels["service"])
	}
	if lines[0].Level != "error" {
		t.Errorf("unexpected level: %s", lines[0].Level)
	}

	// Check nanosecond timestamp conversion
	expected := time.Unix(0, 1708128000000000000).UTC()
	if !lines[0].Timestamp.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, lines[0].Timestamp)
	}

	// Check second line
	expected2 := time.Unix(0, 1708128060000000000).UTC()
	if !lines[1].Timestamp.Equal(expected2) {
		t.Errorf("expected timestamp %v, got %v", expected2, lines[1].Timestamp)
	}
}

func TestQueryRange_MultipleStreams(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := lokiQueryResponse{
			Data: lokiData{
				ResultType: "streams",
				Result: []lokiStream{
					{
						Stream: map[string]string{"service": "api", "level": "error"},
						Values: [][2]string{
							{"1708128000000000000", "error line 1"},
						},
					},
					{
						Stream: map[string]string{"service": "api", "level": "warn"},
						Values: [][2]string{
							{"1708128010000000000", "warn line 1"},
							{"1708128020000000000", "warn line 2"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	lines, err := c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="api"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines from 2 streams, got %d", len(lines))
	}

	// First stream line
	if lines[0].Level != "error" {
		t.Errorf("expected level error, got %s", lines[0].Level)
	}
	// Second stream lines
	if lines[1].Level != "warn" {
		t.Errorf("expected level warn, got %s", lines[1].Level)
	}
}

func TestQueryRange_EmptyResult(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := lokiQueryResponse{
			Data: lokiData{
				ResultType: "streams",
				Result:     []lokiStream{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	lines, err := c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="nonexistent"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if err != nil {
		t.Fatalf("expected no error for empty result, got: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("expected empty slice, got %d lines", len(lines))
	}
}

func TestQueryRange_Loki400_QueryError(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","errorType":"bad_request","error":"parse error"}`))
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	_, err := c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{bad query`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !errors.Is(err, ErrLokiQueryError) {
		t.Errorf("expected ErrLokiQueryError, got: %v", err)
	}
}

func TestQueryRange_Loki500_QueryError(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","error":"internal error"}`))
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	_, err := c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="api"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !errors.Is(err, ErrLokiQueryError) {
		t.Errorf("expected ErrLokiQueryError, got: %v", err)
	}
}

func TestQueryRange_ConnectionRefused(t *testing.T) {
	// Use a URL that can't connect
	c := newTestClient(t, "http://127.0.0.1:1")
	_, err := c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="api"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if !errors.Is(err, ErrLokiUnreachable) {
		t.Errorf("expected ErrLokiUnreachable, got: %v", err)
	}
}

func TestQueryRange_Timeout(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	defer ts.Close()

	// Short timeout client
	c := NewHTTPClient(ts.URL, "", "", "", 100*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.QueryRange(ctx, QueryRangeRequest{
		Query: `{service="api"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !errors.Is(err, ErrLokiTimeout) {
		t.Errorf("expected ErrLokiTimeout, got: %v", err)
	}
}

func TestQueryRange_DirectionParam(t *testing.T) {
	var capturedDirection string
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedDirection = r.URL.Query().Get("direction")
		resp := lokiQueryResponse{Data: lokiData{ResultType: "streams"}}
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	// Default direction
	c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="api"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if capturedDirection != "backward" {
		t.Errorf("expected default direction 'backward', got %q", capturedDirection)
	}

	// Explicit direction
	c.QueryRange(context.Background(), QueryRangeRequest{
		Query:     `{service="api"}`,
		Start:     time.Now().Add(-1 * time.Hour),
		End:       time.Now(),
		Direction: "forward",
	})
	if capturedDirection != "forward" {
		t.Errorf("expected direction 'forward', got %q", capturedDirection)
	}
}

func TestQueryRange_AuthHeaders(t *testing.T) {
	var capturedHeaders http.Header
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		resp := lokiQueryResponse{Data: lokiData{ResultType: "streams"}}
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := NewHTTPClient(ts.URL, "user", "pass", "tenant-1", 5*time.Second)
	c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="api"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})

	if capturedHeaders.Get("X-Scope-OrgID") != "tenant-1" {
		t.Errorf("expected X-Scope-OrgID 'tenant-1', got %q", capturedHeaders.Get("X-Scope-OrgID"))
	}
	// Basic auth
	user, pass, ok := parseBasicAuth(capturedHeaders.Get("Authorization"))
	if !ok || user != "user" || pass != "pass" {
		t.Errorf("expected basic auth user/pass, got %q", capturedHeaders.Get("Authorization"))
	}
}

func TestQueryRange_LevelExtraction(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := lokiQueryResponse{
			Data: lokiData{
				ResultType: "streams",
				Result: []lokiStream{
					{
						Stream: map[string]string{"service": "api", "level": "info"},
						Values: [][2]string{{"1708128000000000000", "info msg"}},
					},
					{
						Stream: map[string]string{"service": "api"},
						Values: [][2]string{{"1708128010000000000", "no level"}},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	lines, err := c.QueryRange(context.Background(), QueryRangeRequest{
		Query: `{service="api"}`,
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lines[0].Level != "info" {
		t.Errorf("expected level 'info', got %q", lines[0].Level)
	}
	if lines[1].Level != "" {
		t.Errorf("expected empty level, got %q", lines[1].Level)
	}
}

// --- Labels tests ---

func TestLabels_Success(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/labels" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := lokiLabelsResponse{
			Status: "success",
			Data:   []string{"service", "namespace", "level"},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	labels, err := c.Labels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(labels))
	}
	if labels[0] != "service" {
		t.Errorf("expected 'service', got %q", labels[0])
	}
}

func TestLabels_Error(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	_, err := c.Labels(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrLokiQueryError) {
		t.Errorf("expected ErrLokiQueryError, got: %v", err)
	}
}

// --- LabelValues tests ---

func TestLabelValues_Success(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/label/service/values" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := lokiLabelsResponse{
			Status: "success",
			Data:   []string{"payments-api", "auth-service"},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	values, err := c.LabelValues(context.Background(), "service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
}

func TestLabelValues_Error(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	_, err := c.LabelValues(context.Background(), "bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Ready tests ---

func TestReady_Success(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	err := c.Ready(context.Background())
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestReady_NotReady(t *testing.T) {
	ts := lokiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	err := c.Ready(context.Background())
	if err == nil {
		t.Fatal("expected error for not ready")
	}
	if !errors.Is(err, ErrLokiUnreachable) {
		t.Errorf("expected ErrLokiUnreachable, got: %v", err)
	}
}

func TestReady_ConnectionRefused(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:1")
	err := c.Ready(context.Background())
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if !errors.Is(err, ErrLokiUnreachable) {
		t.Errorf("expected ErrLokiUnreachable, got: %v", err)
	}
}

// --- helper to parse basic auth ---

func parseBasicAuth(auth string) (string, string, bool) {
	r := &http.Request{Header: http.Header{"Authorization": {auth}}}
	return r.BasicAuth()
}
