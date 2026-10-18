package server

import (
	"errors"
	"net"

	"github.com/jackc/pgx/v5"
)

const (
	// EnvListenAddress names the host:port the process listens on.
	EnvListenAddress = "THREADLINE_CORE_LISTEN_ADDRESS"
	// EnvPostgresDSN names the PostgreSQL connection string. It is a secret and
	// is never echoed in errors or String output.
	EnvPostgresDSN = "THREADLINE_CORE_POSTGRES_DSN"

	defaultListenAddress = "127.0.0.1:8080"
)

// Config is the validated, immutable process configuration.
type Config struct {
	listenAddress string
	postgresDSN   string
}

func (Config) String() string   { return "<redacted-core-config>" }
func (Config) GoString() string { return "<redacted-core-config>" }

// ListenAddress returns the validated host:port to listen on.
func (config Config) ListenAddress() string { return config.listenAddress }

// PostgresDSN returns the connection string for pool construction only.
func (config Config) PostgresDSN() string { return config.postgresDSN }

// LoadConfig reads configuration through lookup, normally os.LookupEnv. It
// validates shape only and never connects.
func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("core config: lookup required")
	}
	listenAddress, found := lookup(EnvListenAddress)
	if !found || listenAddress == "" {
		listenAddress = defaultListenAddress
	}
	if _, _, err := net.SplitHostPort(listenAddress); err != nil {
		return Config{}, errors.New("core config: " + EnvListenAddress + " must be host:port")
	}
	dsn, found := lookup(EnvPostgresDSN)
	if !found || dsn == "" {
		return Config{}, errors.New("core config: " + EnvPostgresDSN + " required")
	}
	if _, err := pgx.ParseConfig(dsn); err != nil {
		return Config{}, errors.New("core config: " + EnvPostgresDSN + " is not a valid PostgreSQL DSN")
	}
	return Config{listenAddress: listenAddress, postgresDSN: dsn}, nil
}
