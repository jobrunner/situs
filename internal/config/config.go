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

// defaultHostusEntryBackbone mirrors hostus.DefaultEntryBackbone, duplicated
// for the same reason as the batch size; a test pins the two together.
const defaultHostusEntryBackbone = "wcvp"

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
	CORS            CORSConfig    `mapstructure:"cors"`
}

// CORSConfig lists the browser origins allowed to call the API from another
// site. Empty (the default) means no CORS headers are sent at all, so the
// service behaves exactly as it does today — correct as long as the only
// browser client is the same-origin explorer under "/".
type CORSConfig struct {
	// AllowedOrigins holds exact origins ("https://example.com") and wildcard
	// patterns ("https://*.example.com"). Set via
	// SITUS_SERVER_CORS_ALLOWED_ORIGINS as a comma-separated list — viper's
	// decode hook splits it, which is why the key needs a registered default
	// (see Defaults below).
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// Configured reports whether any origin string was set at all — it does NOT
// promise CORS will actually work. Config holds strings, nothing more: it has
// no domain.ParseOriginPattern to ask which of them parse, so it cannot know
// whether the middleware ends up wired in. That verdict belongs one layer up,
// after parsing — internal/adapters/http.initCORS is what decides real
// usability (len(s.corsPatterns) > 0), and only that decision should ever be
// called "Enabled". Calling this method "Enabled" claimed the parsed verdict
// from the unparsed input; naming it for what it actually checks — presence,
// not usability — keeps the two truths from being mistaken for each other.
func (c CORSConfig) Configured() bool { return len(c.AllowedOrigins) > 0 }

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
	// EntryBackbone names the hostus taxonomic backbone to match against. A
	// hostus instance built on a different backbone would otherwise leave an
	// operator no lever.
	EntryBackbone string `mapstructure:"entry_backbone"`
}

// Defaults registers every default value.
func Defaults(v *viper.Viper) {
	v.SetDefault("server.host", "127.0.0.1")
	// 8070, nicht 8080: hostus läuft üblicherweise lokal auf 8080, und situs ruft
	// hostus auf — beide Dienste auf demselben Rechner dürfen sich nicht um einen
	// Port streiten.
	v.SetDefault("server.port", 8070)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.shutdown_timeout", 15*time.Second)
	// Registering the key is what lets AutomaticEnv pick up
	// SITUS_SERVER_CORS_ALLOWED_ORIGINS as a comma-separated list; the empty
	// default keeps CORS off.
	v.SetDefault("server.cors.allowed_origins", []string{})

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	v.SetDefault("index.path", "situs.sqlite")

	// 8080 ist hostus' eigener Default-Port; situs weicht deshalb auf 8070 aus
	// (siehe server.port) statt hostus zu verdrängen.
	v.SetDefault("hostus.base_url", "http://localhost:8080")
	v.SetDefault("hostus.timeout", 30*time.Second)
	v.SetDefault("hostus.batch_size", defaultHostusBatchSize)
	v.SetDefault("hostus.entry_backbone", defaultHostusEntryBackbone)
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
