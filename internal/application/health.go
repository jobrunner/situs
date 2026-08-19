// Package application holds the use cases. They orchestrate domain behavior
// through ports and never import an adapter. Later tasks add the ingest and
// query use cases.
package application

import (
	"context"

	"github.com/jobrunner/situs/internal/ports/input"
)

// HealthService answers the readiness probe. Readiness is decided by the
// composition root at construction; once the SQLite index exists (Task 3) it
// becomes a real check against the opened index.
type HealthService struct {
	ready bool
}

// NewHealthService returns a service in the given readiness state.
func NewHealthService(ready bool) *HealthService {
	return &HealthService{ready: ready}
}

// Ready implements input.HealthChecker.
func (s *HealthService) Ready(_ context.Context) bool { return s.ready }

var _ input.HealthChecker = (*HealthService)(nil)
