package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

var ErrNotFound = errors.New("resource not found")
var ErrDuplicateKey = errors.New("duplicate key violation")

// Store is the data access interface. All database operations go through here.
type Store interface {
	Ping(ctx context.Context) error
	GetDefaultTenant(ctx context.Context) (*models.Tenant, error)

	GetAPIKeyByPrefix(ctx context.Context, prefix string) ([]*models.APIKey, error)
	UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error
	CreateAPIKey(ctx context.Context, key *models.APIKey) error
	ListAPIKeys(ctx context.Context, tenantID uuid.UUID) ([]*models.APIKey, error)
	RevokeAPIKey(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error

	UpsertErrorCluster(ctx context.Context, cluster *models.ErrorCluster) (*models.ErrorCluster, error)
	ListErrorClusters(ctx context.Context, filter ClusterFilter) ([]*models.ErrorCluster, int, error)
	GetErrorCluster(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.ErrorCluster, error)
	GetClustersByFingerprints(ctx context.Context, tenantID uuid.UUID, fingerprints []string) ([]*models.ErrorCluster, error)

	CreateAnalysisResult(ctx context.Context, result *models.AnalysisResult) error
	GetAnalysisResultByJobID(ctx context.Context, jobID uuid.UUID) (*models.AnalysisResult, error)
	GetAnalysisResultByClusterID(ctx context.Context, clusterID uuid.UUID) (*models.AnalysisResult, error)

	CreateJob(ctx context.Context, job *models.Job) error
	GetJob(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.Job, error)
	UpdateJobStatus(ctx context.Context, id uuid.UUID, status string, opts ...JobUpdateOption) error
}

type ClusterFilter struct {
	TenantID  uuid.UUID
	Service   string
	Namespace string
	Level     string
	Since     time.Time
	Page      int
	Limit     int
}

type jobUpdateParams struct {
	ErrorMessage *string
	ClusterID    *uuid.UUID
}

type JobUpdateOption func(*jobUpdateParams)

func WithErrorMessage(msg string) JobUpdateOption {
	return func(p *jobUpdateParams) {
		p.ErrorMessage = &msg
	}
}

func WithClusterID(id uuid.UUID) JobUpdateOption {
	return func(p *jobUpdateParams) {
		p.ClusterID = &id
	}
}
