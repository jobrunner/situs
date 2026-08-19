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

// EnvPrefix is the prefix for all environment variables.
const EnvPrefix = "SITUS"

// Config is the whole service configuration. Later tasks add sub-structs per
// concern (index path, hostus endpoint) — one struct per external concern.
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Logging LoggingConfig `mapstructure:"logging"`
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

// Defaults registers every default value.
func Defaults(v *viper.Viper) {
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.shutdown_timeout", 15*time.Second)

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
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
