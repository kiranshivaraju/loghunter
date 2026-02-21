package analysis

import (
	"testing"
	"time"

	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// --- normalizeMessage tests ---

func TestNormalizeMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips leading datetime with T separator",
			input:    "2024-02-17T01:47:32.123Z connection refused",
			expected: "connection refused",
		},
		{
			name:     "strips leading datetime with space separator",
			input:    "2024-02-17 01:47:32 connection refused",
			expected: "connection refused",
		},
		{
			name:     "strips datetime with timezone offset",
			input:    "2024-02-17T01:47:32+05:30 connection refused",
			expected: "connection refused",
		},
		{
			name:     "replaces hex addresses",
			input:    "segfault at 0x7fff5fc00000 in main",
			expected: "segfault at 0xaddr in main",
		},
		{
			name:     "replaces UUIDs",
			input:    "request 550e8400-e29b-41d4-a716-446655440000 failed",
			expected: "request uuid failed",
		},
		{
			name:     "replaces bracketed numbers",
			input:    "goroutine [42] panic",
			expected: "goroutine [n] panic",
		},
		{
			name:     "replaces parenthesized numbers",
			input:    "error code (500) at line (42)",
			expected: "error code (n) at line (n)",
		},
		{
			name:     "collapses whitespace",
			input:    "too   many    spaces",
			expected: "too many spaces",
		},
		{
			name:     "lowercases",
			input:    "Connection REFUSED",
			expected: "connection refused",
		},
		{
			name:     "all normalizations combined",
			input:    "2024-02-17T01:47:32.123Z ERROR at 0xFF addr 550e8400-e29b-41d4-a716-446655440000 goroutine [42]  panic (500)",
			expected: "error at 0xaddr addr uuid goroutine [n] panic (n)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeMessage(tt.input)
			if got != tt.expected {
				t.Errorf("\nexpected: %q\ngot:      %q", tt.expected, got)
			}
		})
	}
}

func TestNormalizeMessage_TruncatesTo500(t *testing.T) {
	long := ""
	for i := 0; i < 600; i++ {
		long += "a"
	}
	got := NormalizeMessage(long)
	if len(got) > 500 {
		t.Errorf("expected max 500 chars, got %d", len(got))
	}
}

// --- Fingerprint tests ---

func TestFingerprint_SameMessageDifferentTimestamps(t *testing.T) {
	fp1 := Fingerprint("2024-02-17T01:00:00Z connection refused to database")
	fp2 := Fingerprint("2024-02-17T02:30:00Z connection refused to database")
	if fp1 != fp2 {
		t.Errorf("same message with different timestamps should have same fingerprint:\n  %s\n  %s", fp1, fp2)
	}
}

func TestFingerprint_SameMessageDifferentUUIDs(t *testing.T) {
	fp1 := Fingerprint("request 550e8400-e29b-41d4-a716-446655440000 failed with error")
	fp2 := Fingerprint("request 123e4567-e89b-12d3-a456-426614174000 failed with error")
	if fp1 != fp2 {
		t.Errorf("same message with different UUIDs should have same fingerprint:\n  %s\n  %s", fp1, fp2)
	}
}

func TestFingerprint_DifferentMessages(t *testing.T) {
	fp1 := Fingerprint("connection refused to database")
	fp2 := Fingerprint("timeout waiting for response")
	if fp1 == fp2 {
		t.Error("different messages should have different fingerprints")
	}
}

func TestFingerprint_IsLowercaseHex(t *testing.T) {
	fp := Fingerprint("test message")
	if len(fp) != 64 {
		t.Errorf("expected 64 char hex string, got %d chars: %s", len(fp), fp)
	}
	for _, c := range fp {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("fingerprint contains non-lowercase-hex char: %c", c)
			break
		}
	}
}

// --- Cluster tests ---

func TestCluster_BasicGrouping(t *testing.T) {
	now := time.Now().UTC()
	lines := []models.LogLine{
		{Timestamp: now.Add(-3 * time.Minute), Message: "connection refused", Level: "error", Labels: map[string]string{}},
		{Timestamp: now.Add(-2 * time.Minute), Message: "connection refused", Level: "error", Labels: map[string]string{}},
		{Timestamp: now.Add(-1 * time.Minute), Message: "connection refused", Level: "error", Labels: map[string]string{}},
		{Timestamp: now, Message: "timeout waiting", Level: "warn", Labels: map[string]string{}},
	}

	clusters := Cluster(lines, "api", "prod")

	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	// First cluster should be "connection refused" (count 3 > count 1)
	if clusters[0].Count != 3 {
		t.Errorf("expected first cluster count 3, got %d", clusters[0].Count)
	}
	if clusters[0].Service != "api" {
		t.Errorf("expected service 'api', got %q", clusters[0].Service)
	}
	if clusters[0].Namespace != "prod" {
		t.Errorf("expected namespace 'prod', got %q", clusters[0].Namespace)
	}

	// Second cluster
	if clusters[1].Count != 1 {
		t.Errorf("expected second cluster count 1, got %d", clusters[1].Count)
	}
}

func TestCluster_FirstSeenLastSeen(t *testing.T) {
	t1 := time.Date(2024, 2, 17, 1, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 2, 17, 1, 5, 0, 0, time.UTC)
	t3 := time.Date(2024, 2, 17, 1, 10, 0, 0, time.UTC)

	lines := []models.LogLine{
		{Timestamp: t2, Message: "error msg", Level: "error", Labels: map[string]string{}},
		{Timestamp: t1, Message: "error msg", Level: "error", Labels: map[string]string{}},
		{Timestamp: t3, Message: "error msg", Level: "error", Labels: map[string]string{}},
	}

	clusters := Cluster(lines, "api", "")
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if !clusters[0].FirstSeenAt.Equal(t1) {
		t.Errorf("expected FirstSeenAt %v, got %v", t1, clusters[0].FirstSeenAt)
	}
	if !clusters[0].LastSeenAt.Equal(t3) {
		t.Errorf("expected LastSeenAt %v, got %v", t3, clusters[0].LastSeenAt)
	}
}

func TestCluster_SeverityOrdering(t *testing.T) {
	now := time.Now().UTC()
	lines := []models.LogLine{
		{Timestamp: now, Message: "warn msg", Level: "warn", Labels: map[string]string{}},
		{Timestamp: now, Message: "fatal msg", Level: "fatal", Labels: map[string]string{}},
		{Timestamp: now, Message: "error msg", Level: "error", Labels: map[string]string{}},
	}

	clusters := Cluster(lines, "api", "")

	// All have count 1, so sort by severity: fatal > error > warn
	if len(clusters) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(clusters))
	}
	if clusters[0].Level != "fatal" {
		t.Errorf("expected first cluster level 'fatal', got %q", clusters[0].Level)
	}
	if clusters[1].Level != "error" {
		t.Errorf("expected second cluster level 'error', got %q", clusters[1].Level)
	}
	if clusters[2].Level != "warn" {
		t.Errorf("expected third cluster level 'warn', got %q", clusters[2].Level)
	}
}

func TestCluster_HighestSeverityWins(t *testing.T) {
	now := time.Now().UTC()
	// Same message but different levels — highest severity should be used
	lines := []models.LogLine{
		{Timestamp: now, Message: "db connection failed", Level: "error", Labels: map[string]string{}},
		{Timestamp: now, Message: "db connection failed", Level: "fatal", Labels: map[string]string{}},
		{Timestamp: now, Message: "db connection failed", Level: "warn", Labels: map[string]string{}},
	}

	clusters := Cluster(lines, "api", "")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Level != "fatal" {
		t.Errorf("expected cluster level 'fatal' (highest severity), got %q", clusters[0].Level)
	}
	if clusters[0].Count != 3 {
		t.Errorf("expected count 3, got %d", clusters[0].Count)
	}
}

func TestCluster_EmptyInput(t *testing.T) {
	clusters := Cluster(nil, "api", "prod")
	if clusters == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(clusters))
	}
}

func TestCluster_EmptySliceInput(t *testing.T) {
	clusters := Cluster([]models.LogLine{}, "api", "prod")
	if clusters == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(clusters))
	}
}

func TestCluster_SampleMessageTruncated(t *testing.T) {
	long := ""
	for i := 0; i < 3000; i++ {
		long += "x"
	}

	lines := []models.LogLine{
		{Timestamp: time.Now(), Message: long, Level: "error", Labels: map[string]string{}},
	}

	clusters := Cluster(lines, "api", "")
	if len(clusters[0].SampleMessage) > 2000 {
		t.Errorf("SampleMessage should be truncated to 2000 chars, got %d", len(clusters[0].SampleMessage))
	}
}

func TestCluster_FingerprintSet(t *testing.T) {
	lines := []models.LogLine{
		{Timestamp: time.Now(), Message: "test error", Level: "error", Labels: map[string]string{}},
	}

	clusters := Cluster(lines, "api", "")
	if clusters[0].Fingerprint == "" {
		t.Error("expected fingerprint to be set")
	}
	if clusters[0].Fingerprint != Fingerprint("test error") {
		t.Errorf("expected fingerprint to match Fingerprint() output")
	}
}

func TestCluster_CountDescThenSeverityDesc(t *testing.T) {
	now := time.Now().UTC()
	lines := []models.LogLine{
		// 2x warn
		{Timestamp: now, Message: "warn msg", Level: "warn", Labels: map[string]string{}},
		{Timestamp: now, Message: "warn msg", Level: "warn", Labels: map[string]string{}},
		// 2x error
		{Timestamp: now, Message: "error msg", Level: "error", Labels: map[string]string{}},
		{Timestamp: now, Message: "error msg", Level: "error", Labels: map[string]string{}},
		// 1x fatal
		{Timestamp: now, Message: "fatal msg", Level: "fatal", Labels: map[string]string{}},
	}

	clusters := Cluster(lines, "api", "")

	if len(clusters) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(clusters))
	}

	// Both count=2, error has higher severity than warn
	if clusters[0].Count != 2 || clusters[0].Level != "error" {
		t.Errorf("expected first: count=2 level=error, got count=%d level=%s", clusters[0].Count, clusters[0].Level)
	}
	if clusters[1].Count != 2 || clusters[1].Level != "warn" {
		t.Errorf("expected second: count=2 level=warn, got count=%d level=%s", clusters[1].Count, clusters[1].Level)
	}
	if clusters[2].Count != 1 || clusters[2].Level != "fatal" {
		t.Errorf("expected third: count=1 level=fatal, got count=%d level=%s", clusters[2].Count, clusters[2].Level)
	}
}

// --- levelSeverity tests ---

func TestLevelSeverity(t *testing.T) {
	tests := []struct {
		level    string
		expected int
	}{
		{"fatal", 4},
		{"FATAL", 4},
		{"critical", 3},
		{"CRITICAL", 3},
		{"error", 2},
		{"ERROR", 2},
		{"warn", 1},
		{"WARN", 1},
		{"warning", 1},
		{"WARNING", 1},
		{"info", 0},
		{"debug", 0},
		{"unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := LevelSeverity(tt.level)
			if got != tt.expected {
				t.Errorf("levelSeverity(%q) = %d, want %d", tt.level, got, tt.expected)
			}
		})
	}
}
