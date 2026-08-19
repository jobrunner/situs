package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jobrunner/situs/internal/adapters/hostus"
	"github.com/jobrunner/situs/internal/config"
)

func TestLoadUsesTheDefaultsWithoutFileOrEnvironment(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") = %v, want no error", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("server.port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ShutdownTimeout != 15*time.Second {
		t.Errorf("server.shutdown_timeout = %v, want 15s", cfg.Server.ShutdownTimeout)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("logging.format = %q, want %q", cfg.Logging.Format, "json")
	}
	if cfg.Hostus.BaseURL != "http://localhost:8081" {
		t.Errorf("hostus.base_url = %q, want the default", cfg.Hostus.BaseURL)
	}
	if cfg.Hostus.Timeout != 30*time.Second {
		t.Errorf("hostus.timeout = %v, want 30s", cfg.Hostus.Timeout)
	}
	if cfg.Hostus.BatchSize != hostus.DefaultBatchSize {
		t.Errorf("hostus.batch_size = %d, want hostus.DefaultBatchSize (%d) — config duplicates the value, so it must not drift",
			cfg.Hostus.BatchSize, hostus.DefaultBatchSize)
	}
	if cfg.Index.Path != "situs.sqlite" {
		t.Errorf("index.path = %q, want the default", cfg.Index.Path)
	}
}

func TestLoadReadsTheIndexPathFromTheEnvironment(t *testing.T) {
	t.Setenv("SITUS_INDEX_PATH", "/srv/situs/index.sqlite")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") = %v, want no error", err)
	}
	if cfg.Index.Path != "/srv/situs/index.sqlite" {
		t.Errorf("index.path = %q, want the value of SITUS_INDEX_PATH", cfg.Index.Path)
	}
}

func TestLoadReadsTheHostusBaseURLFromTheEnvironment(t *testing.T) {
	t.Setenv("SITUS_HOSTUS_BASE_URL", "http://hostus.internal:9000")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") = %v, want no error", err)
	}
	if cfg.Hostus.BaseURL != "http://hostus.internal:9000" {
		t.Errorf("hostus.base_url = %q, want the value of SITUS_HOSTUS_BASE_URL", cfg.Hostus.BaseURL)
	}
}

func TestLoadReadsTheEnvironmentWithTheSitusPrefix(t *testing.T) {
	t.Setenv("SITUS_SERVER_PORT", "9001")
	t.Setenv("SITUS_LOGGING_LEVEL", "debug")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") = %v, want no error", err)
	}
	if cfg.Server.Port != 9001 {
		t.Errorf("server.port = %d, want the value of SITUS_SERVER_PORT", cfg.Server.Port)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("logging.level = %q, want the value of SITUS_LOGGING_LEVEL", cfg.Logging.Level)
	}
}

func TestLoadEnvironmentWinsOverTheConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 7000\n  host: 0.0.0.0\n"), 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}
	t.Setenv("SITUS_SERVER_PORT", "9002")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(%q) = %v, want no error", path, err)
	}
	if cfg.Server.Port != 9002 {
		t.Errorf("server.port = %d, want the environment to win over the file", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("server.host = %q, want the file value", cfg.Server.Host)
	}
}

func TestLoadFailsOnAMissingConfigFile(t *testing.T) {
	if _, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Error("Load(<missing file>) = nil error, want a failure")
	}
}

func TestAddrJoinsHostAndPort(t *testing.T) {
	got := config.ServerConfig{Host: "127.0.0.1", Port: 8080}.Addr()
	if got != "127.0.0.1:8080" {
		t.Errorf("Addr() = %q, want %q", got, "127.0.0.1:8080")
	}
}

func TestLoadReadsTheHostusBatchSizeFromTheEnvironment(t *testing.T) {
	t.Setenv("SITUS_HOSTUS_BATCH_SIZE", "17")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") = %v, want no error", err)
	}
	if cfg.Hostus.BatchSize != 17 {
		t.Errorf("hostus.batch_size = %d, want the value of SITUS_HOSTUS_BATCH_SIZE", cfg.Hostus.BatchSize)
	}
}
