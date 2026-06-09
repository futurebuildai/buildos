package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/futurebuildai/buildos/internal/connectors"
)

func TestBoundTools_DedupsByName(t *testing.T) {
	// A malicious MCP server advertising duplicate names must not 23505 the cache.
	rows := boundTools([]connectors.ToolDef{
		{Name: "search"},
		{Name: "search"}, // duplicate — dropped
		{Name: "fetch"},
		{Name: ""},          // empty — dropped
		{Name: "has space"}, // invalid charset — dropped
	})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (search + fetch, deduped/filtered)", len(rows))
	}
	if rows[0].ToolName != "search" || rows[1].ToolName != "fetch" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestBoundTools_CapsCount(t *testing.T) {
	many := make([]connectors.ToolDef, 100)
	for i := range many {
		many[i] = connectors.ToolDef{Name: "tool_" + string(rune('a'+i%26)) + string(rune('a'+i/26))}
	}
	rows := boundTools(many)
	if len(rows) > maxRefreshTools {
		t.Errorf("got %d rows, want <= %d", len(rows), maxRefreshTools)
	}
}

func TestTruncateUTF8_NeverSplitsRune(t *testing.T) {
	// A description whose multi-byte rune straddles the byte cap must truncate to
	// VALID UTF-8 (a byte-boundary cut would yield invalid bytes a TEXT column
	// rejects, aborting the refresh → a 500).
	s := strings.Repeat("a", maxToolDescBytes-1) + "é€" // multibyte tail across the boundary
	got := truncateUTF8(s, maxToolDescBytes)
	if len(got) > maxToolDescBytes {
		t.Fatalf("len = %d, want <= %d", len(got), maxToolDescBytes)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncated string is not valid UTF-8: %q", got[max(0, len(got)-4):])
	}
	// A short string is returned unchanged.
	if truncateUTF8("short", maxToolDescBytes) != "short" {
		t.Error("a short string must be returned unchanged")
	}
}

func TestBoundTools_RuneSafeDescription(t *testing.T) {
	desc := strings.Repeat("x", maxToolDescBytes-1) + "🚀" // 4-byte rune straddles the cap
	rows := boundTools([]connectors.ToolDef{{Name: "t", Description: desc}})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if !utf8.ValidString(rows[0].Description) {
		t.Error("cached description must be valid UTF-8 (a TEXT column would reject invalid bytes)")
	}
}
