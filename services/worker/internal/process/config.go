// Package process assembles the development Worker; it owns no relay policy.
package process

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

type Config struct {
	address, dsn, broker, user, password, policyFile, owner string
}

func (Config) String() string   { return "<redacted-worker-config>" }
func (Config) GoString() string { return "<redacted-worker-config>" }

func LoadConfig(get func(string) string) (Config, error) {
	c := Config{address: get("THREADLINE_WORKER_LISTEN_ADDRESS"), dsn: get("THREADLINE_WORKER_POSTGRES_DSN"),
		broker: get("THREADLINE_WORKER_NATS_URL"), user: get("THREADLINE_NATS_WORKER_USER"),
		password: get("THREADLINE_NATS_WORKER_PASSWORD"), policyFile: get("THREADLINE_OUTBOX_POLICY_FILE"),
		owner: get("THREADLINE_WORKER_OWNER_ID")}
	if c.address == "" {
		c.address = "127.0.0.1:8081"
	}
	if _, _, err := net.SplitHostPort(c.address); err != nil {
		return Config{}, errors.New("invalid worker listen address")
	}
	u, err := url.Parse(c.broker)
	if err != nil || u.Scheme != "nats" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, errors.New("invalid worker broker address")
	}
	if c.dsn == "" || c.user == "" || c.password == "" || c.policyFile == "" ||
		c.owner == "" || len(c.owner) > 128 || strings.TrimSpace(c.owner) != c.owner {
		return Config{}, errors.New("incomplete worker configuration")
	}
	return c, nil
}
