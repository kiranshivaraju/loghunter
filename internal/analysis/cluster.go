package analysis

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// Normalization regexes compiled once at package init.
var (
	reDatetime   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?\s*`)
	reHexAddr    = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	reUUID       = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reBracketNum = regexp.MustCompile(`\[\d+\]`)
	reParenNum   = regexp.MustCompile(`\(\d+\)`)
	reWhitespace = regexp.MustCompile(`\s+`)
)

// Cluster groups log lines into deduplicated ErrorClusters by fingerprint.
// Returns clusters sorted by (Count DESC, severity DESC).
// Returns empty slice for empty input (never nil).
func Cluster(lines []models.LogLine, service, namespace string) []models.ErrorCluster {
	if len(lines) == 0 {
		return []models.ErrorCluster{}
	}

	type clusterState struct {
		fingerprint   string
		level         string
		count         int
		firstSeen     int64 // unix nano for comparison
		lastSeen      int64
		sampleMessage string
	}

	groups := make(map[string]*clusterState)

	for _, line := range lines {
		fp := Fingerprint(line.Message)
		cs, exists := groups[fp]
		if !exists {
			cs = &clusterState{
				fingerprint:   fp,
				level:         line.Level,
				firstSeen:     line.Timestamp.UnixNano(),
				lastSeen:      line.Timestamp.UnixNano(),
				sampleMessage: truncateString(line.Message, 2000),
			}
			groups[fp] = cs
		}

		cs.count++
		if line.Timestamp.UnixNano() < cs.firstSeen {
			cs.firstSeen = line.Timestamp.UnixNano()
		}
		if line.Timestamp.UnixNano() > cs.lastSeen {
			cs.lastSeen = line.Timestamp.UnixNano()
		}
		if LevelSeverity(line.Level) > LevelSeverity(cs.level) {
			cs.level = line.Level
		}
	}

	clusters := make([]models.ErrorCluster, 0, len(groups))
	for _, cs := range groups {
		clusters = append(clusters, models.ErrorCluster{
			ID:            uuid.New(),
			Service:       service,
			Namespace:     namespace,
			Fingerprint:   cs.fingerprint,
			Level:         cs.level,
			FirstSeenAt:   timeFromNano(cs.firstSeen),
			LastSeenAt:    timeFromNano(cs.lastSeen),
			Count:         cs.count,
			SampleMessage: cs.sampleMessage,
		})
	}

	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Count != clusters[j].Count {
			return clusters[i].Count > clusters[j].Count
		}
		return LevelSeverity(clusters[i].Level) > LevelSeverity(clusters[j].Level)
	})

	return clusters
}

// Fingerprint computes a stable SHA-256 fingerprint for a log message.
func Fingerprint(message string) string {
	normalized := NormalizeMessage(message)
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", hash)
}

// NormalizeMessage applies all normalization rules to a log message.
func NormalizeMessage(msg string) string {
	msg = reDatetime.ReplaceAllString(msg, "")
	msg = reHexAddr.ReplaceAllString(msg, "0xADDR")
	msg = reUUID.ReplaceAllString(msg, "UUID")
	msg = reBracketNum.ReplaceAllString(msg, "[N]")
	msg = reParenNum.ReplaceAllString(msg, "(N)")
	msg = reWhitespace.ReplaceAllString(msg, " ")
	msg = strings.ToLower(msg)
	msg = strings.TrimSpace(msg)
	msg = truncateString(msg, 500)
	return msg
}

// LevelSeverity maps a log level string to a numeric severity.
func LevelSeverity(level string) int {
	switch strings.ToUpper(level) {
	case "FATAL":
		return 4
	case "CRITICAL":
		return 3
	case "ERROR":
		return 2
	case "WARN", "WARNING":
		return 1
	default:
		return 0
	}
}

// truncateString truncates s to maxBytes without splitting UTF-8 runes.
func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

func timeFromNano(ns int64) time.Time {
	return time.Unix(0, ns).UTC()
}
