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

// OpenMemory returns a backend held entirely in memory: nothing is
// created on disk. The schema is not applied; call Migrate. Contents are
// discarded on Close.
func OpenMemory() (*Backend, error) {
	db, err := sql.Open(sqliteDriver, ":memory:")
	if err != nil {
		return nil, fmt.Errorf("open in-memory sqlite: %w", err)
	}
	// Every pooled connection to ":memory:" is its own database; a
	// single connection keeps the schema and rows visible to all calls.
	db.SetMaxOpenConns(1)
	b := &Backend{db: db}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
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
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			name TEXT,
			at_seq_num INTEGER NOT NULL,
			at_timestamp DATETIME NOT NULL,
			data TEXT NOT NULL,
			is_seed INTEGER NOT NULL DEFAULT 0,
			generated_at DATETIME NOT NULL,
			UNIQUE(workspace_id, at_seq_num),
			FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_name ON snapshots(name);`,
		`CREATE INDEX IF NOT EXISTS idx_snapshots_is_seed ON snapshots(is_seed);`,
		`CREATE INDEX IF NOT EXISTS idx_snapshots_workspace ON snapshots(workspace_id);`,
		`CREATE TABLE IF NOT EXISTS spaces (
			id TEXT PRIMARY KEY,
			writer_id TEXT,
			type TEXT NOT NULL,
			parent TEXT,
			boundary TEXT NOT NULL,
			observables TEXT,
			capabilities TEXT NOT NULL,
			trust_level TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY(parent) REFERENCES spaces(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_spaces_parent ON spaces(parent);`,
		`CREATE TABLE IF NOT EXISTS workspace_spaces (
			workspace_id TEXT NOT NULL,
			space_id TEXT NOT NULL,
			PRIMARY KEY(workspace_id, space_id),
			FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
			FOREIGN KEY(space_id) REFERENCES spaces(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_spaces_space ON workspace_spaces(space_id);`,
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

	// OWSF writer and interop fields — safe for existing databases (C2 fix).
	alters := []string{
		`ALTER TABLE events ADD COLUMN writer_id TEXT`,
		`ALTER TABLE events ADD COLUMN writer_seq INTEGER`,
		`ALTER TABLE events ADD COLUMN lamport_clock INTEGER`,
		`ALTER TABLE events ADD COLUMN space_id TEXT`,
		`ALTER TABLE events ADD COLUMN causal_deps TEXT`,
		`ALTER TABLE spaces ADD COLUMN writer_id TEXT`,
		`ALTER TABLE spaces ADD COLUMN observables TEXT`,
		`ALTER TABLE workspaces ADD COLUMN layout TEXT`,
	}
	for _, alter := range alters {
		// Ignore "duplicate column" errors from re-migration.
		if _, err := tx.ExecContext(ctx, alter); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}

	return tx.Commit()
}

func marshal[T any](v T) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// marshalLayout serializes an optional workspace layout; nil stays NULL.
func marshalLayout(l *model.Layout) any {
	if l == nil {
		return nil
	}
	return marshal(l)
}

func (b *Backend) CreateWorkspace(ctx context.Context, ws model.Workspace) error {
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO workspaces(id, name, status, description, tags, metadata, layout, created_at, updated_at, archived_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ws.ID, ws.Name, string(ws.Status), ws.Description, marshal(ws.Tags), marshal(ws.Metadata), marshalLayout(ws.Layout), ws.CreatedAt.UTC(), ws.UpdatedAt.UTC(), ws.ArchivedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("conflict: %w", err)
		}
	}
	return err
}

func scanWorkspace(row scanner) (*model.Workspace, error) {
	var ws model.Workspace
	var tagsJSON, metadataJSON, layoutJSON sql.NullString
	var status string
	var archived sql.NullTime
	if err := row.Scan(&ws.ID, &ws.Name, &status, &ws.Description, &tagsJSON, &metadataJSON, &layoutJSON, &ws.CreatedAt, &ws.UpdatedAt, &archived); err != nil {
		return nil, err
	}
	ws.Status = model.Status(status)
	if tagsJSON.Valid && tagsJSON.String != "" {
		_ = json.Unmarshal([]byte(tagsJSON.String), &ws.Tags)
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		_ = json.Unmarshal([]byte(metadataJSON.String), &ws.Metadata)
	}
	if layoutJSON.Valid && layoutJSON.String != "" && layoutJSON.String != "null" {
		var l model.Layout
		if err := json.Unmarshal([]byte(layoutJSON.String), &l); err == nil {
			ws.Layout = &l
		}
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
		SELECT id, name, status, description, tags, metadata, layout, created_at, updated_at, archived_at
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
		SELECT id, name, status, description, tags, metadata, layout, created_at, updated_at, archived_at
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
		SELECT id, name, status, description, tags, metadata, layout, created_at, updated_at, archived_at
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
		SET name = ?, status = ?, description = ?, tags = ?, metadata = ?, layout = ?, updated_at = ?, archived_at = ?
		WHERE id = ?
	`, ws.Name, string(ws.Status), ws.Description, marshal(ws.Tags), marshal(ws.Metadata), marshalLayout(ws.Layout), ws.UpdatedAt.UTC(), ws.ArchivedAt, ws.ID)
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

	var causalDeps any
	if len(event.CausalDeps) > 0 {
		causalDeps = marshal(event.CausalDeps)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO events(id, workspace_id, type, device_id, session_id, agent_id,
			writer_id, writer_seq, lamport_clock, space_id, causal_deps,
			timestamp, data, seq_num)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.WorkspaceID, string(event.Type), event.DeviceID, event.SessionID, event.AgentID,
		event.WriterID, event.WriterSeq, event.LamportClock, event.SpaceID, causalDeps,
		event.Timestamp.UTC(), string(event.Data), event.SeqNum)
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

const eventColumns = `id, workspace_id, type, device_id, session_id, agent_id,
	writer_id, writer_seq, lamport_clock, space_id, causal_deps,
	timestamp, data, seq_num`

func scanEvent(row scanner) (*model.Event, error) {
	var ev model.Event
	var typ, data string
	var sessionID, agentID, writerID, spaceID, causalDepsJSON sql.NullString
	var writerSeq, lamportClock sql.NullInt64
	if err := row.Scan(
		&ev.ID, &ev.WorkspaceID, &typ, &ev.DeviceID, &sessionID, &agentID,
		&writerID, &writerSeq, &lamportClock, &spaceID, &causalDepsJSON,
		&ev.Timestamp, &data, &ev.SeqNum,
	); err != nil {
		return nil, err
	}
	if sessionID.Valid {
		ev.SessionID = sessionID.String
	}
	if agentID.Valid {
		ev.AgentID = agentID.String
	}
	if writerID.Valid {
		ev.WriterID = writerID.String
	}
	if writerSeq.Valid {
		ev.WriterSeq = uint64(writerSeq.Int64)
	}
	if lamportClock.Valid {
		ev.LamportClock = uint64(lamportClock.Int64)
	}
	if spaceID.Valid {
		ev.SpaceID = spaceID.String
	}
	if causalDepsJSON.Valid && causalDepsJSON.String != "" {
		_ = json.Unmarshal([]byte(causalDepsJSON.String), &ev.CausalDeps)
	}
	ev.Type = model.EventType(typ)
	ev.Data = json.RawMessage(data)
	return &ev, nil
}

func (b *Backend) GetEvent(ctx context.Context, workspaceID string, ref string) (*model.Event, error) {
	var query string
	var arg any
	if seqNum, err := strconv.ParseUint(ref, 10, 64); err == nil {
		query = `SELECT ` + eventColumns + ` FROM events WHERE workspace_id = ? AND seq_num = ?`
		arg = seqNum
	} else {
		query = `SELECT ` + eventColumns + ` FROM events WHERE workspace_id = ? AND id = ?`
		arg = ref
	}
	ev, err := scanEvent(b.db.QueryRowContext(ctx, query, workspaceID, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ev, err
}

func (b *Backend) ListEvents(ctx context.Context, workspaceID string, opts backend.EventListOptions) ([]model.Event, error) {
	query := `SELECT ` + eventColumns + ` FROM events WHERE workspace_id = ?`
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
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ev)
	}
	return out, rows.Err()
}

func (b *Backend) EventCount(ctx context.Context, workspaceID string) (uint64, error) {
	var n uint64
	err := b.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM events WHERE workspace_id = ?`, workspaceID).Scan(&n)
	return n, err
}

func (b *Backend) SaveSnapshot(ctx context.Context, snap model.Snapshot) error {
	isSeed := 0
	if snap.IsSeed {
		isSeed = 1
	}
	var name any
	if snap.Name != "" {
		name = snap.Name
	}

	var existingID string
	err := b.db.QueryRowContext(ctx, `
		SELECT id FROM snapshots WHERE workspace_id = ? AND at_seq_num = ?
	`, snap.WorkspaceID, snap.AtSeqNum).Scan(&existingID)
	if err == nil {
		blob, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		_, err = b.db.ExecContext(ctx, `
			UPDATE snapshots SET name = ?, at_timestamp = ?, data = ?, is_seed = ?, generated_at = ? WHERE id = ?
		`, name, snap.AtTimestamp.UTC(), string(blob), isSeed, snap.GeneratedAt.UTC(), existingID)
		snap.ID = existingID
		return err
	} else if errors.Is(err, sql.ErrNoRows) {
		if snap.ID == "" {
			snap.ID = ulid.Make().String()
		}
		blob, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		_, err = b.db.ExecContext(ctx, `
			INSERT INTO snapshots(id, workspace_id, name, at_seq_num, at_timestamp, data, is_seed, generated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		`, snap.ID, snap.WorkspaceID, name, snap.AtSeqNum, snap.AtTimestamp.UTC(), string(blob), isSeed, snap.GeneratedAt.UTC())
		return err
	}
	return err
}

func (b *Backend) GetSnapshot(ctx context.Context, id string) (*model.Snapshot, error) {
	var data string
	err := b.db.QueryRowContext(ctx, `
		SELECT data FROM snapshots WHERE id = ? OR name = ?
	`, id, id).Scan(&data)
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

func (b *Backend) ListSnapshots(ctx context.Context, opts backend.SnapshotListOptions) ([]model.Snapshot, error) {
	query := "SELECT data FROM snapshots WHERE 1=1"
	var args []any

	if len(opts.WorkspaceIDs) > 0 {
		placeholders := make([]string, len(opts.WorkspaceIDs))
		for i, id := range opts.WorkspaceIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND workspace_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	if !opts.After.IsZero() {
		query += " AND generated_at > ?"
		args = append(args, opts.After.UTC())
	}
	if !opts.Before.IsZero() {
		query += " AND generated_at < ?"
		args = append(args, opts.Before.UTC())
	}

	sortDir := "DESC"
	if opts.SortDir == "asc" {
		sortDir = "ASC"
	}
	query += " ORDER BY generated_at " + sortDir

	rows, err := b.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []model.Snapshot
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var snap model.Snapshot
		if err := json.Unmarshal([]byte(data), &snap); err != nil {
			return nil, err
		}

		if len(opts.Sessions) > 0 {
			if !containsAnySession(snap.Sessions, opts.Sessions) {
				continue
			}
		}

		snaps = append(snaps, snap)
		if opts.Limit > 0 && len(snaps) >= opts.Limit {
			break
		}
	}
	return snaps, rows.Err()
}

func containsAnySession(snapSessions []string, filterSessions []string) bool {
	snapSet := make(map[string]struct{}, len(snapSessions))
	for _, s := range snapSessions {
		snapSet[s] = struct{}{}
	}
	for _, f := range filterSessions {
		if _, ok := snapSet[f]; ok {
			return true
		}
	}
	return false
}

func (b *Backend) UpdateSnapshot(ctx context.Context, snap model.Snapshot) error {
	blob, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	isSeed := 0
	if snap.IsSeed {
		isSeed = 1
	}
	var name any
	if snap.Name != "" {
		name = snap.Name
	}
	result, err := b.db.ExecContext(ctx, `
		UPDATE snapshots SET name = ?, data = ?, is_seed = ? WHERE id = ? OR name = ?
	`, name, string(blob), isSeed, snap.ID, snap.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("snapshot not found: %s", snap.ID)
	}
	return nil
}

func (b *Backend) DeleteSnapshot(ctx context.Context, id string) error {
	result, err := b.db.ExecContext(ctx, `DELETE FROM snapshots WHERE id = ? OR name = ?`, id, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("snapshot not found: %s", id)
	}
	return nil
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

func (b *Backend) ResolveDevice(ctx context.Context, ref string) (*device.Device, []device.Device, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil, fmt.Errorf("device reference is required")
	}

	var d device.Device

	if len(ref) >= 4 {
		err := b.db.QueryRowContext(ctx, `
			SELECT id, name, hostname, os, arch, first_seen, last_seen FROM devices WHERE id = ?
		`, ref).Scan(&d.ID, &d.Name, &d.Hostname, &d.OS, &d.Arch, &d.FirstSeen, &d.LastSeen)
		if err == nil {
			return &d, nil, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}

		rows, err := b.db.QueryContext(ctx, `
			SELECT id, name, hostname, os, arch, first_seen, last_seen FROM devices WHERE id LIKE ? || '%' ORDER BY id
		`, ref)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()

		var matches []device.Device
		for rows.Next() {
			var m device.Device
			if err := rows.Scan(&m.ID, &m.Name, &m.Hostname, &m.OS, &m.Arch, &m.FirstSeen, &m.LastSeen); err != nil {
				return nil, nil, err
			}
			matches = append(matches, m)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}

		if len(matches) == 1 {
			return &matches[0], nil, nil
		}
		if len(matches) > 1 {
			return nil, matches, nil
		}
	}

	rows, err := b.db.QueryContext(ctx, `
		SELECT id, name, hostname, os, arch, first_seen, last_seen FROM devices WHERE name LIKE '%' || ? || '%' ORDER BY name
	`, ref)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var nameMatches []device.Device
	for rows.Next() {
		var m device.Device
		if err := rows.Scan(&m.ID, &m.Name, &m.Hostname, &m.OS, &m.Arch, &m.FirstSeen, &m.LastSeen); err != nil {
			return nil, nil, err
		}
		nameMatches = append(nameMatches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if len(nameMatches) == 1 {
		return &nameMatches[0], nil, nil
	}
	if len(nameMatches) > 1 {
		return nil, nameMatches, nil
	}

	return nil, nil, nil
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

// CreateSpace inserts a space, failing with backend.ErrSpaceExists when the
// ID is already taken. The check is the INSERT itself, so it is atomic.
func (b *Backend) CreateSpace(ctx context.Context, s model.Space) error {
	var parent any
	if s.Parent != "" {
		parent = s.Parent
	}
	var observables any
	if len(s.Observables) > 0 {
		observables = marshal(s.Observables)
	}
	res, err := b.db.ExecContext(ctx, `
		INSERT INTO spaces(id, writer_id, type, parent, boundary, observables, capabilities, trust_level, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, s.ID, s.WriterID, string(s.Type), parent, s.Boundary, observables, marshal(s.Capabilities), string(s.TrustLevel), s.CreatedAt.UTC())
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("create space %q: %w", s.ID, backend.ErrSpaceExists)
	}
	return nil
}

func (b *Backend) SaveSpace(ctx context.Context, s model.Space) error {
	var parent any
	if s.Parent != "" {
		parent = s.Parent
	}
	var observables any
	if len(s.Observables) > 0 {
		observables = marshal(s.Observables)
	}
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO spaces(id, writer_id, type, parent, boundary, observables, capabilities, trust_level, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			writer_id=excluded.writer_id,
			type=excluded.type,
			boundary=excluded.boundary,
			observables=excluded.observables,
			capabilities=excluded.capabilities,
			trust_level=excluded.trust_level
	`, s.ID, s.WriterID, string(s.Type), parent, s.Boundary, observables, marshal(s.Capabilities), string(s.TrustLevel), s.CreatedAt.UTC())
	return err
}

func (b *Backend) GetSpace(ctx context.Context, id string) (*model.Space, error) {
	var s model.Space
	var typ, trustLevel, capsJSON string
	var parent, writerID, observablesJSON sql.NullString
	err := b.db.QueryRowContext(ctx, `
		SELECT id, writer_id, type, parent, boundary, observables, capabilities, trust_level, created_at
		FROM spaces WHERE id = ?
	`, id).Scan(&s.ID, &writerID, &typ, &parent, &s.Boundary, &observablesJSON, &capsJSON, &trustLevel, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Type = model.SpaceType(typ)
	s.TrustLevel = model.TrustLevel(trustLevel)
	if parent.Valid {
		s.Parent = parent.String
	}
	if writerID.Valid {
		s.WriterID = writerID.String
	}
	if observablesJSON.Valid && observablesJSON.String != "" {
		_ = json.Unmarshal([]byte(observablesJSON.String), &s.Observables)
	}
	_ = json.Unmarshal([]byte(capsJSON), &s.Capabilities)
	return &s, nil
}

func (b *Backend) ListSpaces(ctx context.Context, parentID string) ([]model.Space, error) {
	var rows *sql.Rows
	var err error
	query := `SELECT id, writer_id, type, parent, boundary, observables, capabilities, trust_level, created_at FROM spaces`
	if parentID == "" {
		rows, err = b.db.QueryContext(ctx, query+` WHERE parent = '' OR parent IS NULL ORDER BY id`)
	} else {
		rows, err = b.db.QueryContext(ctx, query+` WHERE parent = ? ORDER BY id`, parentID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSpaces(rows)
}

func (b *Backend) DeleteSpace(ctx context.Context, id string, force bool) error {
	// Check for children.
	var childCount int
	if err := b.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM spaces WHERE parent = ?`, id).Scan(&childCount); err != nil {
		return err
	}
	if childCount > 0 && !force {
		return fmt.Errorf("conflict: space %s has %d children; use --force to cascade", id, childCount)
	}
	if childCount > 0 {
		// Recursively delete children first (FK has no ON DELETE CASCADE).
		if _, err := b.db.ExecContext(ctx, `DELETE FROM spaces WHERE parent = ?`, id); err != nil {
			return err
		}
	}
	result, err := b.db.ExecContext(ctx, `DELETE FROM spaces WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("not_found: space %s", id)
	}
	return nil
}

func (b *Backend) AssociateSpace(ctx context.Context, workspaceID, spaceID string) error {
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO workspace_spaces(workspace_id, space_id) VALUES(?, ?)
		ON CONFLICT(workspace_id, space_id) DO NOTHING
	`, workspaceID, spaceID)
	return err
}

func (b *Backend) DissociateSpace(ctx context.Context, workspaceID, spaceID string) error {
	result, err := b.db.ExecContext(ctx, `
		DELETE FROM workspace_spaces WHERE workspace_id = ? AND space_id = ?
	`, workspaceID, spaceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("not_found: association workspace=%s space=%s", workspaceID, spaceID)
	}
	return nil
}

func (b *Backend) ListWorkspaceSpaces(ctx context.Context, workspaceID string) ([]model.Space, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT s.id, s.writer_id, s.type, s.parent, s.boundary, s.observables, s.capabilities, s.trust_level, s.created_at
		FROM spaces s
		JOIN workspace_spaces ws ON ws.space_id = s.id
		WHERE ws.workspace_id = ?
		ORDER BY s.id
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSpaces(rows)
}

func (b *Backend) ListSpaceWorkspaces(ctx context.Context, spaceID string) ([]model.Workspace, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT w.id, w.name, w.status, w.description, w.tags, w.metadata, w.layout, w.created_at, w.updated_at, w.archived_at
		FROM workspaces w
		JOIN workspace_spaces ws ON ws.workspace_id = w.id
		WHERE ws.space_id = ?
		ORDER BY w.name
	`, spaceID)
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

func scanSpaces(rows *sql.Rows) ([]model.Space, error) {
	out := make([]model.Space, 0)
	for rows.Next() {
		var s model.Space
		var typ, trustLevel, capsJSON string
		var parent, writerID, observablesJSON sql.NullString
		if err := rows.Scan(&s.ID, &writerID, &typ, &parent, &s.Boundary, &observablesJSON, &capsJSON, &trustLevel, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Type = model.SpaceType(typ)
		s.TrustLevel = model.TrustLevel(trustLevel)
		if parent.Valid {
			s.Parent = parent.String
		}
		if writerID.Valid {
			s.WriterID = writerID.String
		}
		if observablesJSON.Valid && observablesJSON.String != "" {
			_ = json.Unmarshal([]byte(observablesJSON.String), &s.Observables)
		}
		_ = json.Unmarshal([]byte(capsJSON), &s.Capabilities)
		out = append(out, s)
	}
	return out, rows.Err()
}
