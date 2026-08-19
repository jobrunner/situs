// Package config holds the env-driven service configuration. Keys map to
// environment variables with the SITUS_ prefix: "server.host" is read from
// SITUS_SERVER_HOST. Precedence: env > config file > defaults.
package config

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// defaultHostusBatchSize mirrors hostus.DefaultBatchSize. It is duplicated
// rather than imported because config must not depend on an adapter; a test
// pins the two values to each other.
const defaultHostusBatchSize = 50

// EnvPrefix is the prefix for all environment variables.
const EnvPrefix = "SITUS"

// Config is the whole service configuration. Later tasks add sub-structs per
// concern (index path, hostus endpoint) — one struct per external concern.
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Logging LoggingConfig `mapstructure:"logging"`
	Hostus  HostusConfig  `mapstructure:"hostus"`
	Index   IndexConfig   `mapstructure:"index"`
}

// IndexConfig points at the local SQLite index the read API serves from. It is
// produced by `situs ingest` and opened read-mostly at serve time.
type IndexConfig struct {
	Path string `mapstructure:"path"`
}

// ServerConfig configures the HTTP listener.
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// Addr is the listen address of the HTTP server.
func (c ServerConfig) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// LoggingConfig configures the structured logger.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // debug|info|warn|error
	Format string `mapstructure:"format"` // json|text
}

// HostusConfig configures the name-resolution client used at ingest time
// only — at runtime situs is autark for concept-ID queries.
type HostusConfig struct {
	BaseURL string        `mapstructure:"base_url"`
	Timeout time.Duration `mapstructure:"timeout"`
	// BatchSize is how many verbatim names go into one POST /v1/match. hostus
	// applies a fixed per-request timeout and the cost of a batch depends on its
	// content, so the size that fits is machine-dependent and must be tunable
	// without a recompile.
	BatchSize int `mapstructure:"batch_size"`
}

// Defaults registers every default value.
func Defaults(v *viper.Viper) {
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.shutdown_timeout", 15*time.Second)

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	v.SetDefault("index.path", "situs.sqlite")

	v.SetDefault("hostus.base_url", "http://localhost:8081")
	v.SetDefault("hostus.timeout", 30*time.Second)
	v.SetDefault("hostus.batch_size", defaultHostusBatchSize)
}

// Load merges defaults, an optional config file and the environment.
func Load(configPath string) (*Config, error) {
	v := viper.New()
	Defaults(v)

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // server.host -> SITUS_SERVER_HOST
	v.AutomaticEnv()

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
