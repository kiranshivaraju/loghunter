package logql

import (
	"fmt"
	"strings"
	"time"
)

// QueryBuilder constructs safe LogQL query strings.
// All methods are pure functions with no side effects.
// Zero value is ready to use.
type QueryBuilder struct{}

// DetectionParams defines inputs for error/warning detection queries.
type DetectionParams struct {
	Service   string
	Namespace string
	Start     time.Time
	End       time.Time
	Levels    []string
}

// SearchParams defines inputs for log search queries.
type SearchParams struct {
	Service   string
	Namespace string
	Start     time.Time
	End       time.Time
	Levels    []string
	Keyword   string
}

// BuildDetectionQuery returns a LogQL query for error/warning detection.
func (b QueryBuilder) BuildDetectionQuery(p DetectionParams) string {
	parts := []string{b.buildSelector(p.Service, p.Namespace)}

	if lf := b.buildLevelFilter(p.Levels); lf != "" {
		parts = append(parts, lf)
	}

	return strings.Join(parts, " ")
}

// BuildSearchQuery returns a LogQL query for smart search.
func (b QueryBuilder) BuildSearchQuery(p SearchParams) string {
	parts := []string{b.buildSelector(p.Service, p.Namespace)}

	if kf := b.buildKeywordFilter(p.Keyword); kf != "" {
		parts = append(parts, kf)
	}
	if lf := b.buildLevelFilter(p.Levels); lf != "" {
		parts = append(parts, lf)
	}

	return strings.Join(parts, " ")
}

func (b QueryBuilder) buildSelector(service, namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`{service="%s", namespace="%s"}`, service, namespace)
	}
	return fmt.Sprintf(`{service="%s"}`, service)
}

func (b QueryBuilder) buildLevelFilter(levels []string) string {
	if len(levels) == 0 {
		return ""
	}
	lower := make([]string, len(levels))
	for i, l := range levels {
		lower[i] = strings.ToLower(l)
	}
	return fmt.Sprintf(`| level =~ "(?i)(%s)"`, strings.Join(lower, "|"))
}

func (b QueryBuilder) buildKeywordFilter(keyword string) string {
	if keyword == "" {
		return ""
	}
	return fmt.Sprintf("|= `%s`", keyword)
}
