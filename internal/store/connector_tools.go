package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// ConnectorToolsStore manages the connector_tools cache (Phase 3b-ii): the tools
// fetched from an MCP connector instance's tools/list, refreshed on operator
// demand. ReplaceForConnector atomically swaps the whole tool set for one
// (org, connector). Stateless; methods take pgx.Tx.
type ConnectorToolsStore struct{}

// NewConnectorToolsStore constructs a ConnectorToolsStore.
func NewConnectorToolsStore() *ConnectorToolsStore { return &ConnectorToolsStore{} }

// ConnectorToolRow is one cached tool to persist.
type ConnectorToolRow struct {
	ToolName    string
	Description string
	InputSchema []byte // raw JSONB; nil => "{}"
}

// ReplaceForConnector deletes the existing cached tools for (org, connector) and
// inserts the given set, inside the caller's tx (atomic swap). An empty set
// clears the cache.
func (s *ConnectorToolsStore) ReplaceForConnector(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, connectorName string, tools []ConnectorToolRow) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM connector_tools WHERE org_id = $1 AND connector_name = $2`,
		orgID, connectorName); err != nil {
		return fmt.Errorf("clear connector tools: %w", err)
	}
	for _, t := range tools {
		schema := t.InputSchema
		if schema == nil {
			schema = []byte("{}")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO connector_tools (org_id, connector_name, tool_name, description, input_schema, fetched_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, now())`,
			orgID, connectorName, t.ToolName, t.Description, schema); err != nil {
			return fmt.Errorf("insert connector tool %q: %w", t.ToolName, err)
		}
	}
	return nil
}

// ListByConnector returns the cached tools for (org, connector), ordered by name.
func (s *ConnectorToolsStore) ListByConnector(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, connectorName string) ([]models.ConnectorTool, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, connector_name, tool_name, description, input_schema, fetched_at
		FROM connector_tools
		WHERE org_id = $1 AND connector_name = $2
		ORDER BY tool_name`, orgID, connectorName)
	if err != nil {
		return nil, fmt.Errorf("list connector tools: %w", err)
	}
	defer rows.Close()

	var out []models.ConnectorTool
	for rows.Next() {
		var t models.ConnectorTool
		if err := rows.Scan(&t.ID, &t.OrgID, &t.ConnectorName, &t.ToolName, &t.Description, &t.InputSchema, &t.FetchedAt); err != nil {
			return nil, fmt.Errorf("scan connector tool: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountsByOrg returns tool counts + the latest fetched_at per connector for an
// org, for the admin GET (tools_count / tools_fetched_at).
func (s *ConnectorToolsStore) CountsByOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) (map[string]ConnectorToolStats, error) {
	rows, err := tx.Query(ctx, `
		SELECT connector_name, count(*), max(fetched_at)
		FROM connector_tools
		WHERE org_id = $1
		GROUP BY connector_name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("count connector tools: %w", err)
	}
	defer rows.Close()

	out := make(map[string]ConnectorToolStats)
	for rows.Next() {
		var name string
		var st ConnectorToolStats
		if err := rows.Scan(&name, &st.Count, &st.FetchedAt); err != nil {
			return nil, fmt.Errorf("scan connector tool stats: %w", err)
		}
		out[name] = st
	}
	return out, rows.Err()
}

// ConnectorToolStats is the per-connector cache summary for the admin GET.
type ConnectorToolStats struct {
	Count     int
	FetchedAt time.Time
}
