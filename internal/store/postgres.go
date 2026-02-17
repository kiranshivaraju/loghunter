package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// PostgresStore implements the Store interface using pgx/v5.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Ping checks database connectivity.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// --- Tenants ---

func (s *PostgresStore) GetDefaultTenant(ctx context.Context) (*models.Tenant, error) {
	var t models.Tenant
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, loki_org_id, created_at, updated_at FROM tenants WHERE name = 'default' LIMIT 1`,
	).Scan(&t.ID, &t.Name, &t.LokiOrgID, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get default tenant: %w", err)
	}
	return &t, nil
}

// --- API Keys ---

func (s *PostgresStore) GetAPIKeyByPrefix(ctx context.Context, prefix string) ([]*models.APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, key_hash, key_prefix, scopes, last_used_at, deleted_at, created_at, updated_at
		 FROM api_keys WHERE key_prefix = $1 AND deleted_at IS NULL`, prefix)
	if err != nil {
		return nil, fmt.Errorf("get api key by prefix: %w", err)
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Scopes,
			&k.LastUsedAt, &k.DeletedAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

func (s *PostgresStore) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("update api key last used: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateAPIKey(ctx context.Context, key *models.APIKey) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, tenant_id, name, key_hash, key_prefix, scopes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		key.ID, key.TenantID, key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.CreatedAt, key.UpdatedAt)
	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrDuplicateKey
		}
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListAPIKeys(ctx context.Context, tenantID uuid.UUID) ([]*models.APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, key_hash, key_prefix, scopes, last_used_at, deleted_at, created_at, updated_at
		 FROM api_keys WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Scopes,
			&k.LastUsedAt, &k.DeletedAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

func (s *PostgresStore) RevokeAPIKey(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET deleted_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Error Clusters ---

func (s *PostgresStore) UpsertErrorCluster(ctx context.Context, cluster *models.ErrorCluster) (*models.ErrorCluster, error) {
	var result models.ErrorCluster
	err := s.pool.QueryRow(ctx,
		`INSERT INTO error_clusters (id, tenant_id, service, namespace, fingerprint, level, first_seen_at, last_seen_at, count, sample_message, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (tenant_id, service, namespace, fingerprint) DO UPDATE SET
		   count = error_clusters.count + EXCLUDED.count,
		   last_seen_at = GREATEST(error_clusters.last_seen_at, EXCLUDED.last_seen_at),
		   updated_at = NOW()
		 RETURNING id, tenant_id, service, namespace, fingerprint, level, first_seen_at, last_seen_at, count, sample_message, created_at, updated_at`,
		cluster.ID, cluster.TenantID, cluster.Service, cluster.Namespace, cluster.Fingerprint,
		cluster.Level, cluster.FirstSeenAt, cluster.LastSeenAt, cluster.Count, cluster.SampleMessage,
		cluster.CreatedAt, cluster.UpdatedAt,
	).Scan(&result.ID, &result.TenantID, &result.Service, &result.Namespace, &result.Fingerprint,
		&result.Level, &result.FirstSeenAt, &result.LastSeenAt, &result.Count, &result.SampleMessage,
		&result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert error cluster: %w", err)
	}
	return &result, nil
}

func (s *PostgresStore) ListErrorClusters(ctx context.Context, filter ClusterFilter) ([]*models.ErrorCluster, int, error) {
	// Build WHERE clause dynamically
	conditions := []string{"tenant_id = $1"}
	args := []any{filter.TenantID}
	argIdx := 2

	if filter.Service != "" {
		conditions = append(conditions, fmt.Sprintf("service = $%d", argIdx))
		args = append(args, filter.Service)
		argIdx++
	}
	if filter.Namespace != "" {
		conditions = append(conditions, fmt.Sprintf("namespace = $%d", argIdx))
		args = append(args, filter.Namespace)
		argIdx++
	}
	if filter.Level != "" {
		conditions = append(conditions, fmt.Sprintf("level = $%d", argIdx))
		args = append(args, filter.Level)
		argIdx++
	}
	if !filter.Since.IsZero() {
		conditions = append(conditions, fmt.Sprintf("last_seen_at >= $%d", argIdx))
		args = append(args, filter.Since)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count query
	var total int
	countQuery := "SELECT COUNT(*) FROM error_clusters WHERE " + where
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count error clusters: %w", err)
	}

	// Normalize pagination
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	// Data query
	dataQuery := fmt.Sprintf(
		`SELECT id, tenant_id, service, namespace, fingerprint, level, first_seen_at, last_seen_at, count, sample_message, created_at, updated_at
		 FROM error_clusters WHERE %s ORDER BY last_seen_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list error clusters: %w", err)
	}
	defer rows.Close()

	var clusters []*models.ErrorCluster
	for rows.Next() {
		var c models.ErrorCluster
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Service, &c.Namespace, &c.Fingerprint,
			&c.Level, &c.FirstSeenAt, &c.LastSeenAt, &c.Count, &c.SampleMessage,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan error cluster: %w", err)
		}
		clusters = append(clusters, &c)
	}
	return clusters, total, rows.Err()
}

func (s *PostgresStore) GetErrorCluster(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.ErrorCluster, error) {
	var c models.ErrorCluster
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, service, namespace, fingerprint, level, first_seen_at, last_seen_at, count, sample_message, created_at, updated_at
		 FROM error_clusters WHERE id = $1 AND tenant_id = $2`, id, tenantID,
	).Scan(&c.ID, &c.TenantID, &c.Service, &c.Namespace, &c.Fingerprint,
		&c.Level, &c.FirstSeenAt, &c.LastSeenAt, &c.Count, &c.SampleMessage,
		&c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get error cluster: %w", err)
	}
	return &c, nil
}

func (s *PostgresStore) GetClustersByFingerprints(ctx context.Context, tenantID uuid.UUID, fingerprints []string) ([]*models.ErrorCluster, error) {
	if len(fingerprints) == 0 {
		return []*models.ErrorCluster{}, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, service, namespace, fingerprint, level, first_seen_at, last_seen_at, count, sample_message, created_at, updated_at
		 FROM error_clusters WHERE tenant_id = $1 AND fingerprint = ANY($2)`, tenantID, fingerprints)
	if err != nil {
		return nil, fmt.Errorf("get clusters by fingerprints: %w", err)
	}
	defer rows.Close()

	var clusters []*models.ErrorCluster
	for rows.Next() {
		var c models.ErrorCluster
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Service, &c.Namespace, &c.Fingerprint,
			&c.Level, &c.FirstSeenAt, &c.LastSeenAt, &c.Count, &c.SampleMessage,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan error cluster: %w", err)
		}
		clusters = append(clusters, &c)
	}
	return clusters, rows.Err()
}

// --- Analysis Results ---

func (s *PostgresStore) CreateAnalysisResult(ctx context.Context, result *models.AnalysisResult) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO analysis_results (id, cluster_id, tenant_id, job_id, provider, model, root_cause, confidence, summary, suggested_action, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		result.ID, result.ClusterID, result.TenantID, result.JobID, result.Provider,
		result.Model, result.RootCause, result.Confidence, result.Summary,
		result.SuggestedAction, result.CreatedAt)
	if err != nil {
		return fmt.Errorf("create analysis result: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetAnalysisResultByJobID(ctx context.Context, jobID uuid.UUID) (*models.AnalysisResult, error) {
	var r models.AnalysisResult
	err := s.pool.QueryRow(ctx,
		`SELECT id, cluster_id, tenant_id, job_id, provider, model, root_cause, confidence, summary, suggested_action, created_at
		 FROM analysis_results WHERE job_id = $1`, jobID,
	).Scan(&r.ID, &r.ClusterID, &r.TenantID, &r.JobID, &r.Provider, &r.Model,
		&r.RootCause, &r.Confidence, &r.Summary, &r.SuggestedAction, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get analysis result by job: %w", err)
	}
	return &r, nil
}

func (s *PostgresStore) GetAnalysisResultByClusterID(ctx context.Context, clusterID uuid.UUID) (*models.AnalysisResult, error) {
	var r models.AnalysisResult
	err := s.pool.QueryRow(ctx,
		`SELECT id, cluster_id, tenant_id, job_id, provider, model, root_cause, confidence, summary, suggested_action, created_at
		 FROM analysis_results WHERE cluster_id = $1 ORDER BY created_at DESC LIMIT 1`, clusterID,
	).Scan(&r.ID, &r.ClusterID, &r.TenantID, &r.JobID, &r.Provider, &r.Model,
		&r.RootCause, &r.Confidence, &r.Summary, &r.SuggestedAction, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get analysis result by cluster: %w", err)
	}
	return &r, nil
}

// --- Jobs ---

func (s *PostgresStore) CreateJob(ctx context.Context, job *models.Job) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO jobs (id, tenant_id, type, status, cluster_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		job.ID, job.TenantID, job.Type, job.Status, job.ClusterID, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetJob(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.Job, error) {
	var j models.Job
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, type, status, cluster_id, error_message, started_at, completed_at, created_at, updated_at
		 FROM jobs WHERE id = $1 AND tenant_id = $2`, id, tenantID,
	).Scan(&j.ID, &j.TenantID, &j.Type, &j.Status, &j.ClusterID, &j.ErrorMessage,
		&j.StartedAt, &j.CompletedAt, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return &j, nil
}

var validTransitions = map[string][]string{
	"pending": {"running"},
	"running": {"completed", "failed"},
}

func (s *PostgresStore) UpdateJobStatus(ctx context.Context, id uuid.UUID, status string, opts ...JobUpdateOption) error {
	params := &jobUpdateParams{}
	for _, opt := range opts {
		opt(params)
	}

	// Fetch current status
	var currentStatus string
	err := s.pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1`, id).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get job status: %w", err)
	}

	// Validate transition
	allowed := validTransitions[currentStatus]
	valid := false
	for _, a := range allowed {
		if a == status {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid job status transition: %s -> %s", currentStatus, status)
	}

	now := time.Now().UTC()
	query := `UPDATE jobs SET status = $2, updated_at = $3`
	args := []any{id, status, now}
	argIdx := 4

	if status == "running" {
		query += fmt.Sprintf(", started_at = $%d", argIdx)
		args = append(args, now)
		argIdx++
	}
	if status == "completed" || status == "failed" {
		query += fmt.Sprintf(", completed_at = $%d", argIdx)
		args = append(args, now)
		argIdx++
	}
	if params.ErrorMessage != nil {
		query += fmt.Sprintf(", error_message = $%d", argIdx)
		args = append(args, *params.ErrorMessage)
		argIdx++
	}
	if params.ClusterID != nil {
		query += fmt.Sprintf(", cluster_id = $%d", argIdx)
		args = append(args, *params.ClusterID)
		argIdx++
	}

	query += " WHERE id = $1"

	_, err = s.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

// isDuplicateKeyError checks if a pgx error is a unique constraint violation.
func isDuplicateKeyError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	return false
}
