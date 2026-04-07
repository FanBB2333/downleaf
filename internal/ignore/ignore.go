package ignore

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// defaultPatterns are always active, covering common OS/editor junk files.
var defaultPatterns = []string{
	"Thumbs.db",
	"desktop.ini",
}

// macOSPatterns are macOS-specific dotfiles that pollute Overleaf projects.
var macOSPatterns = []string{
	".DS_Store",
	"._*",
	".Spotlight-V100",
	".Trashes",
}

// pattern represents a single parsed ignore rule.
type pattern struct {
	original  string // raw line (for debugging)
	segments  []string
	negated   bool
	dirOnly   bool
	anchored  bool // pattern contains "/" so it's anchored to root
	matchBase bool // no "/" in pattern → match against basename only
}

// Matcher checks file paths against a list of ignore rules.
type Matcher struct {
	patterns []pattern
}

// New creates a Matcher with default patterns and macOS patterns enabled.
func New() *Matcher {
	return NewWithOptions(true)
}

// NewWithOptions creates a Matcher. If ignoreMacOS is true, macOS-specific
// dotfile patterns (._*, .DS_Store, etc.) are included.
func NewWithOptions(ignoreMacOS bool) *Matcher {
	m := &Matcher{}
	for _, p := range defaultPatterns {
		m.addLine(p)
	}
	if ignoreMacOS {
		for _, p := range macOSPatterns {
			m.addLine(p)
		}
	}
	return m
}

// ParseFile reads a .dlignore file and returns a Matcher.
// Built-in defaults are prepended; user rules from the file are appended.
// If the file does not exist, only defaults are returned (no error).
// ignoreMacOS controls whether macOS dotfile patterns are included.
func ParseFile(path string, ignoreMacOS bool) (*Matcher, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewWithOptions(ignoreMacOS), nil
		}
		return nil, err
	}
	defer f.Close()
	return ParseReaderWithOptions(f, ignoreMacOS), nil
}

// ParseReader reads ignore rules from a reader.
// Built-in defaults and macOS patterns are prepended.
func ParseReader(r io.Reader) *Matcher {
	return ParseReaderWithOptions(r, true)
}

// ParseReaderWithOptions reads ignore rules from a reader.
// Built-in defaults are prepended; ignoreMacOS controls macOS patterns.
func ParseReaderWithOptions(r io.Reader, ignoreMacOS bool) *Matcher {
	m := NewWithOptions(ignoreMacOS)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		m.addLine(scanner.Text())
	}
	return m
}

func (m *Matcher) addLine(line string) {
	line = strings.TrimRight(line, "\r")
	// Strip trailing whitespace (unless escaped with backslash)
	if !strings.HasSuffix(line, "\\ ") {
		line = strings.TrimRight(line, " \t")
	}
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}

	p := pattern{original: line}

	if strings.HasPrefix(line, "!") {
		p.negated = true
		line = line[1:]
	}

	// Leading "/" means anchored to root; strip it for matching
	if strings.HasPrefix(line, "/") {
		p.anchored = true
		line = line[1:]
	}

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// If the pattern contains no "/" (after stripping leading/trailing),
	// it matches against the basename only
	if !strings.Contains(line, "/") && !p.anchored {
		p.matchBase = true
	}

	p.segments = strings.Split(line, "/")
	m.patterns = append(m.patterns, p)
}

// Match returns true if the given path should be ignored.
// path should be a forward-slash-separated relative path (no leading "/").
// isDir indicates whether the path refers to a directory.
func (m *Matcher) Match(filePath string, isDir bool) bool {
	filePath = filepath.ToSlash(filePath)
	filePath = strings.TrimPrefix(filePath, "/")

	ignored := false
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if matchPattern(p, filePath) {
			ignored = !p.negated
		}
	}
	return ignored
}

// matchPattern tests if a single pattern matches the given path.
func matchPattern(p pattern, filePath string) bool {
	if p.matchBase {
		// Match against basename only
		base := filepath.Base(filePath)
		return globMatch(p.segments[0], base)
	}

	pathParts := strings.Split(filePath, "/")

	if p.anchored {
		// Must match from root
		return matchSegments(p.segments, pathParts)
	}

	// Unanchored pattern with "/" — try matching from every directory level.
	// We must try all start positions because "**" can absorb variable depths.
	for i := 0; i < len(pathParts); i++ {
		if matchSegments(p.segments, pathParts[i:]) {
			return true
		}
	}
	return false
}

// matchSegments matches pattern segments against path parts, supporting "**".
func matchSegments(patSegs, pathParts []string) bool {
	pi, pp := 0, 0
	for pi < len(patSegs) && pp < len(pathParts) {
		if patSegs[pi] == "**" {
			// "**" at end matches everything remaining
			if pi == len(patSegs)-1 {
				return true
			}
			// Try matching the rest of pattern at every remaining position
			for k := pp; k <= len(pathParts); k++ {
				if matchSegments(patSegs[pi+1:], pathParts[k:]) {
					return true
				}
			}
			return false
		}
		if !globMatch(patSegs[pi], pathParts[pp]) {
			return false
		}
		pi++
		pp++
	}

	// All remaining pattern segments must be "**" to still match
	for pi < len(patSegs) {
		if patSegs[pi] != "**" {
			return false
		}
		pi++
	}
	return pp == len(pathParts)
}

// globMatch matches a single segment using filepath.Match semantics.
func globMatch(pattern, name string) bool {
	matched, err := filepath.Match(pattern, name)
	if err != nil {
		return false
	}
	return matched
}
