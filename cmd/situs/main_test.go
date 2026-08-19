package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/config"
)

func TestVersionCommandPrintsTheBuildInformation(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("executing version: %v", err)
	}
	if !strings.Contains(out.String(), Version) {
		t.Errorf("output = %q, want it to contain the version %q", out.String(), Version)
	}
}

func TestRootCommandKnowsServeAndVersion(t *testing.T) {
	got := map[string]bool{}
	for _, c := range newRootCmd().Commands() {
		got[c.Name()] = true
	}
	for _, want := range []string{"serve", "version"} {
		if !got[want] {
			t.Errorf("subcommand %q missing, have %v", want, got)
		}
	}
}

func TestLoggerFormatAndLevelFollowTheConfig(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cfg        config.LoggingConfig
		logAt      slog.Level
		wantOutput string
	}{
		{
			name:       "json is the default format",
			cfg:        config.LoggingConfig{Level: "info", Format: "json"},
			logAt:      slog.LevelInfo,
			wantOutput: `"msg":"hello"`,
		},
		{
			name:       "text format",
			cfg:        config.LoggingConfig{Level: "info", Format: "text"},
			logAt:      slog.LevelInfo,
			wantOutput: "msg=hello",
		},
		{
			name:       "an unparseable level falls back to info",
			cfg:        config.LoggingConfig{Level: "bogus", Format: "text"},
			logAt:      slog.LevelInfo,
			wantOutput: "msg=hello",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			setupLogger(tc.cfg, &buf).Log(t.Context(), tc.logAt, "hello")

			if !strings.Contains(buf.String(), tc.wantOutput) {
				t.Errorf("log = %q, want it to contain %q", buf.String(), tc.wantOutput)
			}
		})
	}
}

func TestLoggerSuppressesBelowTheConfiguredLevel(t *testing.T) {
	var buf bytes.Buffer
	setupLogger(config.LoggingConfig{Level: "error", Format: "text"}, &buf).Info("hello")

	if buf.Len() != 0 {
		t.Errorf("log = %q, want nothing below the configured level", buf.String())
	}
}
