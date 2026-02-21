package loki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// Sentinel errors for Loki client failures.
var (
	ErrLokiUnreachable = errors.New("loki unreachable")
	ErrLokiQueryError  = errors.New("loki query error")
	ErrLokiTimeout     = errors.New("loki query timeout")
)

// Client is the interface for querying Loki.
type Client interface {
	QueryRange(ctx context.Context, req QueryRangeRequest) ([]models.LogLine, error)
	Labels(ctx context.Context) ([]string, error)
	LabelValues(ctx context.Context, label string) ([]string, error)
	Ready(ctx context.Context) error
}

// QueryRangeRequest defines parameters for a Loki range query.
type QueryRangeRequest struct {
	Query     string
	Start     time.Time
	End       time.Time
	Limit     int
	Direction string
}

// HTTPClient implements Client using Loki's HTTP API.
type HTTPClient struct {
	baseURL  string
	username string
	password string
	orgID    string
	client   *http.Client
}

// NewHTTPClient creates a new Loki HTTP client.
func NewHTTPClient(baseURL, username, password, orgID string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		orgID:    orgID,
		client:   &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) QueryRange(ctx context.Context, req QueryRangeRequest) ([]models.LogLine, error) {
	direction := req.Direction
	if direction == "" {
		direction = "backward"
	}

	params := url.Values{
		"query":     {req.Query},
		"start":     {strconv.FormatInt(req.Start.UnixNano(), 10)},
		"end":       {strconv.FormatInt(req.End.UnixNano(), 10)},
		"direction": {direction},
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}

	u := fmt.Sprintf("%s/loki/api/v1/query_range?%s", c.baseURL, params.Encode())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, classifyError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrLokiQueryError, resp.StatusCode)
	}

	var lokiResp lokiQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&lokiResp); err != nil {
		return nil, fmt.Errorf("decoding loki response: %w", err)
	}

	return parseStreams(lokiResp.Data.Result), nil
}

func (c *HTTPClient) Labels(ctx context.Context) ([]string, error) {
	u := fmt.Sprintf("%s/loki/api/v1/labels", c.baseURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, classifyError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrLokiQueryError, resp.StatusCode)
	}

	var labelsResp lokiLabelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&labelsResp); err != nil {
		return nil, fmt.Errorf("decoding labels response: %w", err)
	}

	return labelsResp.Data, nil
}

func (c *HTTPClient) LabelValues(ctx context.Context, label string) ([]string, error) {
	u := fmt.Sprintf("%s/loki/api/v1/label/%s/values", c.baseURL, url.PathEscape(label))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, classifyError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrLokiQueryError, resp.StatusCode)
	}

	var valuesResp lokiLabelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&valuesResp); err != nil {
		return nil, fmt.Errorf("decoding label values response: %w", err)
	}

	return valuesResp.Data, nil
}

func (c *HTTPClient) Ready(ctx context.Context) error {
	u := fmt.Sprintf("%s/ready", c.baseURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLokiUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: loki not ready (status %d)", ErrLokiUnreachable, resp.StatusCode)
	}

	return nil
}

func (c *HTTPClient) setHeaders(req *http.Request) {
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	if c.orgID != "" {
		req.Header.Set("X-Scope-OrgID", c.orgID)
	}
}

// classifyError maps transport-level errors to sentinel errors.
func classifyError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %v", ErrLokiTimeout, err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("%w: %v", ErrLokiTimeout, err)
		}
		return fmt.Errorf("%w: %v", ErrLokiUnreachable, err)
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Errorf("%w: %v", ErrLokiUnreachable, err)
	}

	return fmt.Errorf("%w: %v", ErrLokiUnreachable, err)
}

// parseStreams converts Loki stream results into LogLine slices.
func parseStreams(streams []lokiStream) []models.LogLine {
	var lines []models.LogLine
	for _, stream := range streams {
		level := stream.Stream["level"]
		for _, v := range stream.Values {
			ts, _ := strconv.ParseInt(v[0], 10, 64)
			lines = append(lines, models.LogLine{
				Timestamp: time.Unix(0, ts).UTC(),
				Message:   v[1],
				Labels:    stream.Stream,
				Level:     level,
			})
		}
	}
	if lines == nil {
		return []models.LogLine{}
	}
	return lines
}

// --- Loki response types ---

type lokiQueryResponse struct {
	Data lokiData `json:"data"`
}

type lokiData struct {
	ResultType string       `json:"resultType"`
	Result     []lokiStream `json:"result"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiLabelsResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
}

// Compile-time check that HTTPClient implements Client.
var _ Client = (*HTTPClient)(nil)
