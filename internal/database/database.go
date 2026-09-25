package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/thinkelution/open-ffmpeg-transcoder/migrations"
)

type DB struct {
	conn *sql.DB
}

func New(databaseURL string) (*DB, error) {
	conn, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) Migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for i, fname := range upFiles {
		version := i + 1

		var count int
		err := db.conn.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", version).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrations.FS.ReadFile(fname)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", fname, err)
		}

		tx, err := db.conn.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", fname, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}

	return nil
}

// --- Job CRUD ---

func (db *DB) CreateJob(ctx context.Context, req *CreateJobRequest) (*Job, error) {
	inputJSON, _ := json.Marshal(req.Input)
	outputJSON, _ := json.Marshal(req.Output)
	settingsJSON, _ := json.Marshal(req.Settings)

	if req.Priority == 0 {
		req.Priority = 5
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	job := &Job{}
	err := db.conn.QueryRowContext(ctx, `
		INSERT INTO jobs (input_config, output_config, settings, priority, webhook_url, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, status, progress, speed, fps, eta, input_config, output_config, settings,
		          priority, webhook_url, metadata, error_message, input_info, output_info,
		          started_at, completed_at, duration_ms, created_at, updated_at
	`, inputJSON, outputJSON, settingsJSON, req.Priority, req.WebhookURL, metadata,
	).Scan(
		&job.ID, &job.Status, &job.Progress, &job.Speed, &job.FPS, &job.ETA,
		&job.InputConfig, &job.OutputConfig, &job.Settings,
		&job.Priority, &job.WebhookURL, &job.Metadata,
		&job.ErrorMessage, &job.InputInfo, &job.OutputInfo,
		&job.StartedAt, &job.CompletedAt, &job.DurationMs,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return job, nil
}

func (db *DB) GetJob(ctx context.Context, id string) (*Job, error) {
	job := &Job{}
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, status, progress, speed, fps, eta, input_config, output_config, settings,
		       priority, webhook_url, metadata, error_message, input_info, output_info,
		       started_at, completed_at, duration_ms, created_at, updated_at
		FROM jobs WHERE id = $1
	`, id).Scan(
		&job.ID, &job.Status, &job.Progress, &job.Speed, &job.FPS, &job.ETA,
		&job.InputConfig, &job.OutputConfig, &job.Settings,
		&job.Priority, &job.WebhookURL, &job.Metadata,
		&job.ErrorMessage, &job.InputInfo, &job.OutputInfo,
		&job.StartedAt, &job.CompletedAt, &job.DurationMs,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return job, nil
}

func (db *DB) ListJobs(ctx context.Context, params JobListParams) ([]*Job, int, error) {
	if params.Limit == 0 {
		params.Limit = 50
	}

	where := ""
	args := []interface{}{}
	argIdx := 1

	if params.Status != "" {
		where = fmt.Sprintf(" WHERE status = $%d", argIdx)
		args = append(args, params.Status)
		argIdx++
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM jobs" + where
	if err := db.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count jobs: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, status, progress, speed, fps, eta, input_config, output_config, settings,
		       priority, webhook_url, metadata, error_message, input_info, output_info,
		       started_at, completed_at, duration_ms, created_at, updated_at
		FROM jobs%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, params.Limit, params.Offset)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		job := &Job{}
		if err := rows.Scan(
			&job.ID, &job.Status, &job.Progress, &job.Speed, &job.FPS, &job.ETA,
			&job.InputConfig, &job.OutputConfig, &job.Settings,
			&job.Priority, &job.WebhookURL, &job.Metadata,
			&job.ErrorMessage, &job.InputInfo, &job.OutputInfo,
			&job.StartedAt, &job.CompletedAt, &job.DurationMs,
			&job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, total, nil
}

func (db *DB) UpdateJobStatus(ctx context.Context, id string, status JobStatus) error {
	extra := ""
	switch status {
	case JobStatusDownloading, JobStatusTranscoding:
		extra = ", started_at = COALESCE(started_at, NOW())"
	case JobStatusCompleted, JobStatusFailed, JobStatusCancelled:
		extra = ", completed_at = NOW(), duration_ms = EXTRACT(EPOCH FROM (NOW() - COALESCE(started_at, created_at))) * 1000"
	}
	_, err := db.conn.ExecContext(ctx,
		fmt.Sprintf("UPDATE jobs SET status = $1, updated_at = NOW()%s WHERE id = $2", extra),
		status, id,
	)
	return err
}

func (db *DB) UpdateJobProgress(ctx context.Context, id string, progress float32, speed string, fps float32) error {
	_, err := db.conn.ExecContext(ctx,
		"UPDATE jobs SET progress = $1, speed = $2, fps = $3, updated_at = NOW() WHERE id = $4",
		progress, speed, fps, id,
	)
	return err
}

func (db *DB) UpdateJobError(ctx context.Context, id string, errMsg string) error {
	_, err := db.conn.ExecContext(ctx,
		"UPDATE jobs SET status = 'failed', error_message = $1, completed_at = NOW(), duration_ms = EXTRACT(EPOCH FROM (NOW() - COALESCE(started_at, created_at))) * 1000, updated_at = NOW() WHERE id = $2",
		errMsg, id,
	)
	return err
}

func (db *DB) UpdateJobInputInfo(ctx context.Context, id string, info json.RawMessage) error {
	_, err := db.conn.ExecContext(ctx,
		"UPDATE jobs SET input_info = $1, updated_at = NOW() WHERE id = $2",
		info, id,
	)
	return err
}

func (db *DB) UpdateJobOutputInfo(ctx context.Context, id string, info json.RawMessage) error {
	_, err := db.conn.ExecContext(ctx,
		"UPDATE jobs SET output_info = $1, updated_at = NOW() WHERE id = $2",
		info, id,
	)
	return err
}

func (db *DB) DeleteJob(ctx context.Context, id string) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM jobs WHERE id = $1", id)
	return err
}

func (db *DB) HasScannerJobForSource(ctx context.Context, bucket, key string) (bool, error) {
	var exists bool
	err := db.conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM jobs
			WHERE metadata->>'scanner_bucket' = $1
			  AND metadata->>'scanner_source_key' = $2
			  AND status <> 'failed'
		)
	`, bucket, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check scanner source job: %w", err)
	}
	return exists, nil
}

func (db *DB) GetJobCounts(ctx context.Context) (map[string]int, error) {
	rows, err := db.conn.QueryContext(ctx, "SELECT status, COUNT(*) FROM jobs GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, nil
}

// --- Preset CRUD ---

func (db *DB) CreatePreset(ctx context.Context, req *CreatePresetRequest) (*Preset, error) {
	preset := &Preset{}
	err := db.conn.QueryRowContext(ctx, `
		INSERT INTO presets (name, description, settings)
		VALUES ($1, $2, $3)
		RETURNING id, name, description, settings, is_system, created_at, updated_at
	`, req.Name, req.Description, req.Settings,
	).Scan(&preset.ID, &preset.Name, &preset.Description, &preset.Settings, &preset.IsSystem, &preset.CreatedAt, &preset.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create preset: %w", err)
	}
	return preset, nil
}

func (db *DB) GetPreset(ctx context.Context, id string) (*Preset, error) {
	preset := &Preset{}
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, name, description, settings, is_system, created_at, updated_at
		FROM presets WHERE id = $1
	`, id).Scan(&preset.ID, &preset.Name, &preset.Description, &preset.Settings, &preset.IsSystem, &preset.CreatedAt, &preset.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get preset: %w", err)
	}
	return preset, nil
}

func (db *DB) ListPresets(ctx context.Context) ([]*Preset, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, name, description, settings, is_system, created_at, updated_at
		FROM presets ORDER BY is_system DESC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	defer rows.Close()

	var presets []*Preset
	for rows.Next() {
		p := &Preset{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Settings, &p.IsSystem, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}
		presets = append(presets, p)
	}
	return presets, nil
}

func (db *DB) UpdatePreset(ctx context.Context, id string, req *UpdatePresetRequest) (*Preset, error) {
	sets := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != "" {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, req.Name)
		argIdx++
	}
	if req.Description != "" {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, req.Description)
		argIdx++
	}
	if req.Settings != nil {
		sets = append(sets, fmt.Sprintf("settings = $%d", argIdx))
		args = append(args, req.Settings)
		argIdx++
	}

	if len(sets) == 0 {
		return db.GetPreset(ctx, id)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE presets SET %s WHERE id = $%d AND is_system = FALSE
		RETURNING id, name, description, settings, is_system, created_at, updated_at
	`, strings.Join(sets, ", "), argIdx)

	preset := &Preset{}
	err := db.conn.QueryRowContext(ctx, query, args...).Scan(
		&preset.ID, &preset.Name, &preset.Description, &preset.Settings, &preset.IsSystem, &preset.CreatedAt, &preset.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update preset: %w", err)
	}
	return preset, nil
}

func (db *DB) DeletePreset(ctx context.Context, id string) error {
	result, err := db.conn.ExecContext(ctx, "DELETE FROM presets WHERE id = $1 AND is_system = FALSE", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("preset not found or is a system preset")
	}
	return nil
}
