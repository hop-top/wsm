package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/oklog/ulid/v2"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/device"
	"hop.top/wsm/pkg/model"
)

const sqliteDriver = "sqlite3"

// Backend implements backend.Backend using SQLite.
type Backend struct {
	db *sql.DB
}

var _ backend.Backend = (*Backend)(nil)

func Open(path string) (*Backend, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite data dir: %w", err)
	}
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	b := &Backend{db: db}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable wal mode: %w", err)
	}
	return b, nil
}

func (b *Backend) Close() error { return b.db.Close() }

func (b *Backend) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'active',
			description TEXT,
			tags TEXT,
			metadata TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			archived_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			type TEXT NOT NULL,
			device_id TEXT,
			session_id TEXT,
			agent_id TEXT,
			timestamp DATETIME NOT NULL,
			data TEXT NOT NULL,
			seq_num INTEGER NOT NULL,
			UNIQUE(workspace_id, seq_num),
			FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_events_workspace_seq ON events(workspace_id, seq_num);`,
		`CREATE INDEX IF NOT EXISTS idx_events_workspace_type ON events(workspace_id, type);`,
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			hostname TEXT,
			os TEXT,
			arch TEXT,
			first_seen DATETIME NOT NULL,
			last_seen DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			at_seq_num INTEGER NOT NULL,
			at_timestamp DATETIME NOT NULL,
			data TEXT NOT NULL,
			generated_at DATETIME NOT NULL,
			UNIQUE(workspace_id, at_seq_num),
			FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		);`,
	}

	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func marshal[T any](v T) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (b *Backend) CreateWorkspace(ctx context.Context, ws model.Workspace) error {
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO workspaces(id, name, status, description, tags, metadata, created_at, updated_at, archived_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ws.ID, ws.Name, string(ws.Status), ws.Description, marshal(ws.Tags), marshal(ws.Metadata), ws.CreatedAt.UTC(), ws.UpdatedAt.UTC(), ws.ArchivedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("conflict: %w", err)
		}
	}
	return err
}

func scanWorkspace(row scanner) (*model.Workspace, error) {
	var ws model.Workspace
	var tagsJSON, metadataJSON sql.NullString
	var status string
	var archived sql.NullTime
	if err := row.Scan(&ws.ID, &ws.Name, &status, &ws.Description, &tagsJSON, &metadataJSON, &ws.CreatedAt, &ws.UpdatedAt, &archived); err != nil {
		return nil, err
	}
	ws.Status = model.Status(status)
	if tagsJSON.Valid && tagsJSON.String != "" {
		_ = json.Unmarshal([]byte(tagsJSON.String), &ws.Tags)
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		_ = json.Unmarshal([]byte(metadataJSON.String), &ws.Metadata)
	}
	if archived.Valid {
		t := archived.Time
		ws.ArchivedAt = &t
	}
	return &ws, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (b *Backend) GetWorkspace(ctx context.Context, id string) (*model.Workspace, error) {
	row := b.db.QueryRowContext(ctx, `
		SELECT id, name, status, description, tags, metadata, created_at, updated_at, archived_at
		FROM workspaces WHERE id = ?
	`, id)
	ws, err := scanWorkspace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ws, err
}

func (b *Backend) GetWorkspaceByName(ctx context.Context, name string) (*model.Workspace, error) {
	row := b.db.QueryRowContext(ctx, `
		SELECT id, name, status, description, tags, metadata, created_at, updated_at, archived_at
		FROM workspaces WHERE name = ?
	`, name)
	ws, err := scanWorkspace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ws, err
}

func (b *Backend) ListWorkspaces(ctx context.Context, opts backend.ListOptions) ([]model.Workspace, error) {
	sortBy := "created_at"
	switch opts.SortBy {
	case "name", "updated_at", "created_at", "created", "updated":
		sortBy = opts.SortBy
	}
	if sortBy == "created" {
		sortBy = "created_at"
	}
	if sortBy == "updated" {
		sortBy = "updated_at"
	}
	sortDir := "DESC"
	if strings.EqualFold(opts.SortDir, "asc") {
		sortDir = "ASC"
	}
	query := `
		SELECT id, name, status, description, tags, metadata, created_at, updated_at, archived_at
		FROM workspaces
	`
	args := make([]any, 0, 3)
	if opts.Status != nil {
		query += " WHERE status = ?"
		args = append(args, string(*opts.Status))
	}
	query += " ORDER BY " + sortBy + " " + sortDir
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}
	rows, err := b.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Workspace, 0)
	for rows.Next() {
		ws, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ws)
	}
	return out, rows.Err()
}

func (b *Backend) UpdateWorkspace(ctx context.Context, ws model.Workspace) error {
	_, err := b.db.ExecContext(ctx, `
		UPDATE workspaces
		SET name = ?, status = ?, description = ?, tags = ?, metadata = ?, updated_at = ?, archived_at = ?
		WHERE id = ?
	`, ws.Name, string(ws.Status), ws.Description, marshal(ws.Tags), marshal(ws.Metadata), ws.UpdatedAt.UTC(), ws.ArchivedAt, ws.ID)
	return err
}

func (b *Backend) DeleteWorkspace(ctx context.Context, id string) error {
	_, err := b.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, id)
	return err
}

func (b *Backend) AppendEvent(ctx context.Context, event model.Event) (model.Event, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Event{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM workspaces WHERE id = ?`, event.WorkspaceID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Event{}, fmt.Errorf("not_found: workspace")
		}
		return model.Event{}, err
	}
	if status == string(model.StatusArchived) &&
		event.Type != model.EventWorkspaceArchived &&
		event.Type != model.EventWorkspaceUnarchived {
		return model.Event{}, fmt.Errorf("conflict: workspace archived")
	}

	if event.ID == "" {
		event.ID = ulid.Make().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.SeqNum == 0 {
		var next uint64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq_num), 0) + 1 FROM events WHERE workspace_id = ?`, event.WorkspaceID).Scan(&next); err != nil {
			return model.Event{}, err
		}
		event.SeqNum = next
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO events(id, workspace_id, type, device_id, session_id, agent_id, timestamp, data, seq_num)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.WorkspaceID, string(event.Type), event.DeviceID, event.SessionID, event.AgentID, event.Timestamp.UTC(), string(event.Data), event.SeqNum)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return model.Event{}, fmt.Errorf("conflict: %w", err)
		}
		return model.Event{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET updated_at = ? WHERE id = ?`, event.Timestamp.UTC(), event.WorkspaceID); err != nil {
		return model.Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Event{}, err
	}
	return event, nil
}

func (b *Backend) GetEvent(ctx context.Context, workspaceID string, ref string) (*model.Event, error) {
	var query string
	var arg any
	if seqNum, err := strconv.ParseUint(ref, 10, 64); err == nil {
		query = `SELECT id, workspace_id, type, device_id, session_id, agent_id, timestamp, data, seq_num FROM events WHERE workspace_id = ? AND seq_num = ?`
		arg = seqNum
	} else {
		query = `SELECT id, workspace_id, type, device_id, session_id, agent_id, timestamp, data, seq_num FROM events WHERE workspace_id = ? AND id = ?`
		arg = ref
	}
	var ev model.Event
	var typ, data string
	var sessionID, agentID sql.NullString
	err := b.db.QueryRowContext(ctx, query, workspaceID, arg).Scan(
		&ev.ID, &ev.WorkspaceID, &typ, &ev.DeviceID, &sessionID, &agentID, &ev.Timestamp, &data, &ev.SeqNum,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sessionID.Valid {
		ev.SessionID = sessionID.String
	}
	if agentID.Valid {
		ev.AgentID = agentID.String
	}
	ev.Type = model.EventType(typ)
	ev.Data = json.RawMessage(data)
	return &ev, nil
}

func (b *Backend) ListEvents(ctx context.Context, workspaceID string, opts backend.EventListOptions) ([]model.Event, error) {
	query := `SELECT id, workspace_id, type, device_id, session_id, agent_id, timestamp, data, seq_num FROM events WHERE workspace_id = ?`
	args := []any{workspaceID}
	if opts.AfterSeqNum > 0 {
		query += " AND seq_num > ?"
		args = append(args, opts.AfterSeqNum)
	}
	if opts.BeforeSeqNum > 0 {
		query += " AND seq_num < ?"
		args = append(args, opts.BeforeSeqNum)
	}
	if len(opts.Types) > 0 {
		query += " AND type IN ("
		for i, t := range opts.Types {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, string(t))
		}
		query += ")"
	}
	query += " ORDER BY seq_num ASC"
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	rows, err := b.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Event, 0)
	for rows.Next() {
		var ev model.Event
		var typ string
		var data string
		var sessionID, agentID sql.NullString
		if err := rows.Scan(&ev.ID, &ev.WorkspaceID, &typ, &ev.DeviceID, &sessionID, &agentID, &ev.Timestamp, &data, &ev.SeqNum); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			ev.SessionID = sessionID.String
		}
		if agentID.Valid {
			ev.AgentID = agentID.String
		}
		ev.Type = model.EventType(typ)
		ev.Data = json.RawMessage(data)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (b *Backend) EventCount(ctx context.Context, workspaceID string) (uint64, error) {
	var n uint64
	err := b.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM events WHERE workspace_id = ?`, workspaceID).Scan(&n)
	return n, err
}

func (b *Backend) SaveSnapshot(ctx context.Context, snap model.Snapshot) error {
	blob, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = b.db.ExecContext(ctx, `
		INSERT INTO snapshots(workspace_id, at_seq_num, at_timestamp, data, generated_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, at_seq_num)
		DO UPDATE SET at_timestamp = excluded.at_timestamp, data = excluded.data, generated_at = excluded.generated_at
	`, snap.WorkspaceID, snap.AtSeqNum, snap.AtTimestamp.UTC(), string(blob), snap.GeneratedAt.UTC())
	return err
}

func (b *Backend) GetLatestSnapshot(ctx context.Context, workspaceID string) (*model.Snapshot, error) {
	var data string
	err := b.db.QueryRowContext(ctx, `
		SELECT data FROM snapshots WHERE workspace_id = ? ORDER BY at_seq_num DESC LIMIT 1
	`, workspaceID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snap model.Snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (b *Backend) SnapshotCount(ctx context.Context, workspaceID string) (uint64, error) {
	var n uint64
	err := b.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM snapshots WHERE workspace_id = ?`, workspaceID).Scan(&n)
	return n, err
}

func (b *Backend) SaveDevice(ctx context.Context, dev device.Device) error {
	if dev.FirstSeen.IsZero() {
		dev.FirstSeen = time.Now().UTC()
	}
	if dev.LastSeen.IsZero() {
		dev.LastSeen = dev.FirstSeen
	}
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO devices(id, name, hostname, os, arch, first_seen, last_seen)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			hostname=excluded.hostname,
			os=excluded.os,
			arch=excluded.arch,
			last_seen=excluded.last_seen
	`, dev.ID, dev.Name, dev.Hostname, dev.OS, dev.Arch, dev.FirstSeen.UTC(), dev.LastSeen.UTC())
	return err
}

func (b *Backend) GetDevice(ctx context.Context, id string) (*device.Device, error) {
	var d device.Device
	err := b.db.QueryRowContext(ctx, `
		SELECT id, name, hostname, os, arch, first_seen, last_seen FROM devices WHERE id = ?
	`, id).Scan(&d.ID, &d.Name, &d.Hostname, &d.OS, &d.Arch, &d.FirstSeen, &d.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (b *Backend) ListDevices(ctx context.Context) ([]device.Device, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, name, hostname, os, arch, first_seen, last_seen FROM devices ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]device.Device, 0)
	for rows.Next() {
		var d device.Device
		if err := rows.Scan(&d.ID, &d.Name, &d.Hostname, &d.OS, &d.Arch, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (b *Backend) ListDeviceSessions(ctx context.Context, deviceID string) ([]backend.DeviceSession, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT
			e.device_id,
			COALESCE(e.session_id, '') AS session_id,
			COALESCE(e.agent_id, '') AS agent_id,
			e.workspace_id,
			w.name AS workspace_name,
			COUNT(*) AS event_count,
			MAX(e.timestamp) AS last_event_at
		FROM events e
		JOIN workspaces w ON w.id = e.workspace_id
		WHERE e.device_id = ?
		GROUP BY e.device_id, e.session_id, e.agent_id, e.workspace_id
		ORDER BY last_event_at DESC
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]backend.DeviceSession, 0)
	for rows.Next() {
		var ds backend.DeviceSession
		var lastEventAt string
		if err := rows.Scan(
			&ds.DeviceID, &ds.SessionID,
			&ds.AgentID, &ds.WorkspaceID,
			&ds.WorkspaceName, &ds.EventCount,
			&lastEventAt,
		); err != nil {
			return nil, err
		}
		ds.LastEventAt, _ = time.Parse(time.RFC3339Nano, lastEventAt)
		if ds.LastEventAt.IsZero() {
			ds.LastEventAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", lastEventAt)
		}
		if ds.LastEventAt.IsZero() {
			ds.LastEventAt, _ = time.Parse("2006-01-02T15:04:05Z", lastEventAt)
		}
		out = append(out, ds)
	}
	return out, rows.Err()
}
