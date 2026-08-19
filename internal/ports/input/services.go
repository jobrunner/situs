// Package input holds the driving ports — what the application offers to
// primary adapters. The HTTP adapter depends on these interfaces, never on
// concrete application services. Task 8 adds QueryService here.
package input

import "context"

// HealthChecker backs the readiness probe: /health/ready must report NOT ready
// until the index and every dependency are usable, so an orchestrator does not
// route traffic prematurely.
type HealthChecker interface {
	Ready(ctx context.Context) bool
}
