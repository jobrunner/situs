package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/application"
)

func TestHealthServiceReportsConfiguredReadiness(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ready bool
	}{
		{name: "ready", ready: true},
		{name: "not ready", ready: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := application.NewHealthService(tc.ready)
			if got := svc.Ready(context.Background()); got != tc.ready {
				t.Errorf("Ready() = %v, want %v", got, tc.ready)
			}
		})
	}
}
