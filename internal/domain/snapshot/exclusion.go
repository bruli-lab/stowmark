package snapshot

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
)

const IgnoreFileName = ".stowmarkignore"

var ErrInvalidExcludePattern = errors.New("invalid exclude pattern")

const anySegments = "**"

type excludePattern struct {
	base          []string
	segments      []string
	negated       bool
	directoryOnly bool
}

type ExcludeRules struct {
	ignoreFiles      []excludePattern
	overrides        []excludePattern
	skipsIgnoreFiles bool
}

func NewExcludeRules(patterns []string) (*ExcludeRules, error) {
	parsed, err := parseExcludePatterns(nil, patterns)
	if err != nil {
		return nil, err
	}
	return &ExcludeRules{ignoreFiles: parsed}, nil
}

func (r *ExcludeRules) WithOverrides(patterns []string) (*ExcludeRules, error) {
	parsed, err := parseExcludePatterns(nil, patterns)
	if err != nil {
		return nil, err
	}
	rules := r.clone()
	rules.overrides = append(rules.overrides, parsed...)
	return rules, nil
}

func (r *ExcludeRules) WithIgnoreFile(directory string, lines []string) (*ExcludeRules, error) {
	var base []string
	if directory != "" && directory != "." {
		base = strings.Split(directory, "/")
	}
	parsed, err := parseExcludePatterns(base, lines)
	if err != nil {
		return nil, fmt.Errorf("%s in %q: %w", IgnoreFileName, directory, err)
	}
	if len(parsed) == 0 {
		return r, nil
	}
	rules := r.clone()
	rules.ignoreFiles = append(rules.ignoreFiles, parsed...)
	return rules, nil
}

func (r *ExcludeRules) WithoutIgnoreFiles() *ExcludeRules {
	rules := r.clone()
	rules.skipsIgnoreFiles = true
	return rules
}

func (r *ExcludeRules) ReadsIgnoreFiles() bool {
	return !r.skipsIgnoreFiles
}

func (r *ExcludeRules) clone() *ExcludeRules {
	return &ExcludeRules{
		ignoreFiles:      slices.Clone(r.ignoreFiles),
		overrides:        slices.Clone(r.overrides),
		skipsIgnoreFiles: r.skipsIgnoreFiles,
	}
}

func parseExcludePatterns(base, lines []string) ([]excludePattern, error) {
	patterns := make([]excludePattern, 0, len(lines))
	for _, raw := range lines {
		pattern, ok, err := parseExcludePattern(raw)
		if err != nil {
			return nil, err
		}
		if ok {
			pattern.base = base
			patterns = append(patterns, pattern)
		}
	}
	return patterns, nil
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
	for _, patterns := range [][]excludePattern{r.ignoreFiles, r.overrides} {
		for _, pattern := range patterns {
			if matched, ok := pattern.matches(name, isDir); ok {
				excluded = matched
			}
		}
	}
	return excluded
}

func (p excludePattern) matches(name []string, isDir bool) (excluded, matched bool) {
	if p.directoryOnly && !isDir {
		return false, false
	}
	if len(name) <= len(p.base) || !slices.Equal(name[:len(p.base)], p.base) {
		return false, false
	}
	if !matchSegments(p.segments, name[len(p.base):]) {
		return false, false
	}
	return !p.negated, true
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
