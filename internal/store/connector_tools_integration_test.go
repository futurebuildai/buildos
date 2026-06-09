//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

func TestConnectorToolsStore_ReplaceListCount(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewConnectorToolsStore()
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Tools Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// First refresh: two tools.
		if err := s.ReplaceForConnector(ctx, tx, orgID, "acme", []ConnectorToolRow{
			{ToolName: "search", Description: "find", InputSchema: []byte(`{"type":"object"}`)},
			{ToolName: "fetch", Description: "get"},
		}); err != nil {
			return err
		}
		got, err := s.ListByConnector(ctx, tx, orgID, "acme")
		if err != nil {
			return err
		}
		if len(got) != 2 || got[0].ToolName != "fetch" || got[1].ToolName != "search" { // ORDER BY tool_name
			t.Fatalf("tools = %+v", got)
		}
		if string(got[1].InputSchema) != `{"type": "object"}` && string(got[1].InputSchema) != `{"type":"object"}` {
			t.Errorf("schema round-trip = %s", got[1].InputSchema)
		}

		// Second refresh replaces the set (one tool).
		if err := s.ReplaceForConnector(ctx, tx, orgID, "acme", []ConnectorToolRow{{ToolName: "only"}}); err != nil {
			return err
		}
		got2, err := s.ListByConnector(ctx, tx, orgID, "acme")
		if err != nil {
			return err
		}
		if len(got2) != 1 || got2[0].ToolName != "only" {
			t.Fatalf("after replace, tools = %+v, want only the new one", got2)
		}

		counts, err := s.CountsByOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if counts["acme"].Count != 1 {
			t.Errorf("count = %d, want 1", counts["acme"].Count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestConnectorToolsStore_OrgIsolation(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewConnectorToolsStore()
	ctx := context.Background()
	orgA, orgB := uuid.New(), uuid.New()
	testdb.SeedOrg(t, pool, orgA, "A")
	testdb.SeedOrg(t, pool, orgB, "B")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.ReplaceForConnector(ctx, tx, orgA, "acme", []ConnectorToolRow{{ToolName: "x"}}); err != nil {
			return err
		}
		got, err := s.ListByConnector(ctx, tx, orgB, "acme")
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Errorf("org B must not see org A's cached tools, got %d", len(got))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
