package snapshot

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

const IgnoreFileName = ".stowmarkignore"

var ErrInvalidExcludePattern = errors.New("invalid exclude pattern")

const anySegments = "**"

type excludePattern struct {
	segments      []string
	negated       bool
	directoryOnly bool
}

type ExcludeRules struct {
	patterns []excludePattern
}

func NewExcludeRules(patterns []string) (*ExcludeRules, error) {
	rules := &ExcludeRules{patterns: make([]excludePattern, 0, len(patterns))}
	for _, raw := range patterns {
		pattern, ok, err := parseExcludePattern(raw)
		if err != nil {
			return nil, err
		}
		if ok {
			rules.patterns = append(rules.patterns, pattern)
		}
	}
	return rules, nil
}

func parseExcludePattern(raw string) (excludePattern, bool, error) {
	line := strings.TrimRight(raw, " \t\r")
	if line == "" || strings.HasPrefix(line, "#") {
		return excludePattern{}, false, nil
	}

	pattern := excludePattern{}
	if rest, ok := strings.CutPrefix(line, "!"); ok {
		pattern.negated = true
		line = rest
	}
	line = strings.TrimPrefix(line, `\`)
	if rest, ok := strings.CutSuffix(line, "/"); ok {
		pattern.directoryOnly = true
		line = rest
	}
	anchored := strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")
	if line == "" {
		return excludePattern{}, false, fmt.Errorf("%w: %q", ErrInvalidExcludePattern, raw)
	}

	segments := strings.Split(line, "/")
	for _, segment := range segments {
		if segment == "" {
			return excludePattern{}, false, fmt.Errorf("%w: %q", ErrInvalidExcludePattern, raw)
		}
		if _, err := path.Match(segment, ""); err != nil {
			return excludePattern{}, false, fmt.Errorf("%w: %q: %w", ErrInvalidExcludePattern, raw, err)
		}
	}
	if !anchored {
		segments = append([]string{anySegments}, segments...)
	}
	pattern.segments = segments

	return pattern, true, nil
}

func (r *ExcludeRules) Excludes(relativePath string, isDir bool) bool {
	name := strings.Split(relativePath, "/")
	excluded := false
	for _, pattern := range r.patterns {
		if pattern.directoryOnly && !isDir {
			continue
		}
		if matchSegments(pattern.segments, name) {
			excluded = !pattern.negated
		}
	}
	return excluded
}

func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == anySegments {
		if len(pattern) == 1 {
			return len(name) > 0
		}
		for i := range len(name) + 1 {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], name[0])
	return err == nil && matched && matchSegments(pattern[1:], name[1:])
}
