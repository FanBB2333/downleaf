package ignore

import (
	"strings"
	"testing"
)

func TestBlankAndCommentLines(t *testing.T) {
	m := ParseReader(strings.NewReader("# comment\n\n   \n"))
	// Only default patterns should be active; a random file is not ignored
	if m.Match("hello.tex", false) {
		t.Error("hello.tex should not be ignored")
	}
}

func TestDefaultPatterns(t *testing.T) {
	m := New()
	cases := []struct {
		path string
		want bool
	}{
		{".DS_Store", true},
		{"subdir/.DS_Store", true},
		{"._foo", true},
		{"deep/nested/._bar", true},
		{"Thumbs.db", true},
		{"desktop.ini", true},
		{".Spotlight-V100", true},
		{".Trashes", true},
		{"main.tex", false},
		{"refs.bib", false},
	}
	for _, tc := range cases {
		got := m.Match(tc.path, false)
		if got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestSimpleGlob(t *testing.T) {
	m := ParseReader(strings.NewReader("*.log\nbuild/\n"))
	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"error.log", false, true},
		{"subdir/debug.log", false, true},
		{"build", true, true},
		{"build", false, false}, // dir-only rule
		{"src/build", true, true},
		{"main.tex", false, false},
	}
	for _, tc := range cases {
		got := m.Match(tc.path, tc.isDir)
		if got != tc.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tc.path, tc.isDir, got, tc.want)
		}
	}
}

func TestNegation(t *testing.T) {
	m := ParseReader(strings.NewReader("*.log\n!important.log\n"))
	if !m.Match("error.log", false) {
		t.Error("error.log should be ignored")
	}
	if m.Match("important.log", false) {
		t.Error("important.log should NOT be ignored (negated)")
	}
}

func TestAnchoredPattern(t *testing.T) {
	m := ParseReader(strings.NewReader("/output\n"))
	if !m.Match("output", false) {
		t.Error("root output should be ignored")
	}
	if m.Match("sub/output", false) {
		t.Error("nested output should NOT be ignored (anchored to root)")
	}
}

func TestDoubleStarPattern(t *testing.T) {
	m := ParseReader(strings.NewReader("**/logs\nlogs/**\nsrc/**/temp\n"))
	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"logs", false, true},
		{"a/logs", false, true},
		{"a/b/logs", false, true},
		{"logs/debug.log", false, true},
		{"logs/sub/trace.log", false, true},
		{"src/temp", false, true},
		{"src/a/b/temp", false, true},
		{"other/temp", false, false},
	}
	for _, tc := range cases {
		got := m.Match(tc.path, tc.isDir)
		if got != tc.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tc.path, tc.isDir, got, tc.want)
		}
	}
}

func TestPathWithSlash(t *testing.T) {
	m := ParseReader(strings.NewReader("doc/generated\n"))
	if !m.Match("doc/generated", false) {
		t.Error("doc/generated should be ignored")
	}
	if !m.Match("prefix/doc/generated", false) {
		t.Error("prefix/doc/generated should be ignored (unanchored pattern with /)")
	}
}

func TestParseFileMissing(t *testing.T) {
	m, err := ParseFile("/nonexistent/.dlignore")
	if err != nil {
		t.Fatalf("ParseFile should not error on missing file: %v", err)
	}
	// Should still have defaults
	if !m.Match(".DS_Store", false) {
		t.Error(".DS_Store should be ignored by defaults")
	}
}
