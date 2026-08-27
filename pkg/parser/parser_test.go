package parser

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func keys(m map[string]*repository) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

// Renovate can emit a single JSON log line larger than any fixed buffer size: a
// repository that matches hundreds of files dumps a multi-megabyte debug line. Parsing
// such a line must not error or panic, and parsing must continue with the lines that
// follow it. Previously bufio.Scanner returned bufio.ErrTooLong here, which panicked the
// whole process (cmd/main.go).
func TestParseHandlesOversizedLine(t *testing.T) {
	// A huge debug line that does not match any of the parsed message types (so it is
	// skipped), far larger than the deliberately tiny BufferSize below.
	huge := `{"repository":"org/huge","level":20,"msg":"` + strings.Repeat("x", 200000) + `"}`
	// A normal line that is parsed; it contains RepositoryFinishedMessage.
	small := `{"repository":"org/small","time":"2026-01-01T00:00:00Z","msg":"` + RepositoryFinishedMessage + `"}`
	input := huge + "\n" + small + "\n"

	p := NewParser(strings.NewReader(input), ParserOptions{
		BufferSize: 64,
		Logger:     logr.Discard(),
	})

	repos, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if _, ok := repos["org/small"]; !ok {
		t.Fatalf("Parse() repos = %v, want to contain %q (line after the oversized line)", keys(repos), "org/small")
	}
}

// The final line must be processed even when the stream has no trailing newline.
func TestParseFinalLineWithoutTrailingNewline(t *testing.T) {
	input := `{"repository":"org/only","time":"2026-01-01T00:00:00Z","msg":"` + RepositoryFinishedMessage + `"}`

	p := NewParser(strings.NewReader(input), ParserOptions{
		BufferSize: 64,
		Logger:     logr.Discard(),
	})

	repos, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if _, ok := repos["org/only"]; !ok {
		t.Fatalf("Parse() repos = %v, want to contain %q (final line without a trailing newline)", keys(repos), "org/only")
	}
}
