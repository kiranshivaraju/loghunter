package logql

import "testing"

func TestBuildDetectionQuery(t *testing.T) {
	b := QueryBuilder{}

	tests := []struct {
		name     string
		params   DetectionParams
		expected string
	}{
		{
			name: "service with namespace and single level",
			params: DetectionParams{
				Service:   "payments-api",
				Namespace: "production",
				Levels:    []string{"ERROR"},
			},
			expected: `{service="payments-api", namespace="production"} | level =~ "(?i)(error)"`,
		},
		{
			name: "service with namespace and multiple levels",
			params: DetectionParams{
				Service:   "payments-api",
				Namespace: "production",
				Levels:    []string{"ERROR", "FATAL"},
			},
			expected: `{service="payments-api", namespace="production"} | level =~ "(?i)(error|fatal)"`,
		},
		{
			name: "namespace omitted when empty",
			params: DetectionParams{
				Service: "api",
				Levels:  []string{"ERROR"},
			},
			expected: `{service="api"} | level =~ "(?i)(error)"`,
		},
		{
			name: "multiple levels including WARN and WARNING",
			params: DetectionParams{
				Service:   "api",
				Namespace: "staging",
				Levels:    []string{"WARN", "WARNING"},
			},
			expected: `{service="api", namespace="staging"} | level =~ "(?i)(warn|warning)"`,
		},
		{
			name: "no levels - no level filter",
			params: DetectionParams{
				Service:   "api",
				Namespace: "prod",
			},
			expected: `{service="api", namespace="prod"}`,
		},
		{
			name: "three levels",
			params: DetectionParams{
				Service: "gateway",
				Levels:  []string{"ERROR", "FATAL", "CRITICAL"},
			},
			expected: `{service="gateway"} | level =~ "(?i)(error|fatal|critical)"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.BuildDetectionQuery(tt.params)
			if got != tt.expected {
				t.Errorf("\nexpected: %s\ngot:      %s", tt.expected, got)
			}
		})
	}
}

func TestBuildSearchQuery(t *testing.T) {
	b := QueryBuilder{}

	tests := []struct {
		name     string
		params   SearchParams
		expected string
	}{
		{
			name: "keyword with levels",
			params: SearchParams{
				Service: "payments-api",
				Keyword: "timeout",
				Levels:  []string{"ERROR", "WARN"},
			},
			expected: "{service=\"payments-api\"} |= `timeout` | level =~ \"(?i)(error|warn)\"",
		},
		{
			name: "keyword without levels",
			params: SearchParams{
				Service: "api",
				Keyword: "connection refused",
			},
			expected: "{service=\"api\"} |= `connection refused`",
		},
		{
			name: "empty keyword - no line filter",
			params: SearchParams{
				Service: "api",
				Levels:  []string{"ERROR"},
			},
			expected: `{service="api"} | level =~ "(?i)(error)"`,
		},
		{
			name: "keyword with namespace",
			params: SearchParams{
				Service:   "api",
				Namespace: "production",
				Keyword:   "panic",
				Levels:    []string{"FATAL"},
			},
			expected: "{service=\"api\", namespace=\"production\"} |= `panic` | level =~ \"(?i)(fatal)\"",
		},
		{
			name: "no keyword no levels - selector only",
			params: SearchParams{
				Service:   "api",
				Namespace: "prod",
			},
			expected: `{service="api", namespace="prod"}`,
		},
		{
			name: "keyword with special characters",
			params: SearchParams{
				Service: "api",
				Keyword: `error "code":500`,
			},
			expected: "{service=\"api\"} |= `error \"code\":500`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.BuildSearchQuery(tt.params)
			if got != tt.expected {
				t.Errorf("\nexpected: %s\ngot:      %s", tt.expected, got)
			}
		})
	}
}

func TestBuildSelector(t *testing.T) {
	b := QueryBuilder{}

	tests := []struct {
		name      string
		service   string
		namespace string
		expected  string
	}{
		{
			name:      "with namespace",
			service:   "payments-api",
			namespace: "production",
			expected:  `{service="payments-api", namespace="production"}`,
		},
		{
			name:     "without namespace",
			service:  "api",
			expected: `{service="api"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.buildSelector(tt.service, tt.namespace)
			if got != tt.expected {
				t.Errorf("\nexpected: %s\ngot:      %s", tt.expected, got)
			}
		})
	}
}

func TestBuildLevelFilter(t *testing.T) {
	b := QueryBuilder{}

	tests := []struct {
		name     string
		levels   []string
		expected string
	}{
		{
			name:     "single level",
			levels:   []string{"ERROR"},
			expected: `| level =~ "(?i)(error)"`,
		},
		{
			name:     "multiple levels",
			levels:   []string{"ERROR", "FATAL"},
			expected: `| level =~ "(?i)(error|fatal)"`,
		},
		{
			name:     "empty levels",
			levels:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.buildLevelFilter(tt.levels)
			if got != tt.expected {
				t.Errorf("\nexpected: %q\ngot:      %q", tt.expected, got)
			}
		})
	}
}

func TestBuildKeywordFilter(t *testing.T) {
	b := QueryBuilder{}

	tests := []struct {
		name     string
		keyword  string
		expected string
	}{
		{
			name:     "simple keyword",
			keyword:  "timeout",
			expected: "|= `timeout`",
		},
		{
			name:     "keyword with spaces",
			keyword:  "connection refused",
			expected: "|= `connection refused`",
		},
		{
			name:     "empty keyword",
			keyword:  "",
			expected: "",
		},
		{
			name:     "keyword with quotes",
			keyword:  `error "fatal"`,
			expected: "|= `error \"fatal\"`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.buildKeywordFilter(tt.keyword)
			if got != tt.expected {
				t.Errorf("\nexpected: %q\ngot:      %q", tt.expected, got)
			}
		})
	}
}

func TestQueryBuilder_ZeroValue(t *testing.T) {
	// Zero-value QueryBuilder should work without initialization
	var b QueryBuilder
	got := b.BuildDetectionQuery(DetectionParams{
		Service: "test",
		Levels:  []string{"ERROR"},
	})
	expected := `{service="test"} | level =~ "(?i)(error)"`
	if got != expected {
		t.Errorf("zero-value builder failed:\nexpected: %s\ngot:      %s", expected, got)
	}
}
