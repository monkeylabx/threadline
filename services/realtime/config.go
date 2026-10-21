package main

import (
	"errors"
	"net/url"
	"strings"
)

type config struct{ address, core, broker, user, password string }

func (config) String() string   { return "<redacted-realtime-config>" }
func (config) GoString() string { return "<redacted-realtime-config>" }

func load(get func(string) string) (config, error) {
	c := config{get("THREADLINE_REALTIME_LISTEN_ADDRESS"), get("THREADLINE_REALTIME_CORE_URL"),
		get("THREADLINE_REALTIME_NATS_URL"), get("THREADLINE_NATS_REALTIME_USER"), get("THREADLINE_NATS_REALTIME_PASSWORD")}
	if c.address == "" {
		c.address = "127.0.0.1:8082"
	}
	if !endpoint(c.core, "http", "https") || !endpoint(c.broker, "nats") || c.user == "" || c.password == "" {
		return config{}, errors.New("invalid realtime configuration")
	}
	return c, nil
}

func endpoint(value string, schemes ...string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Trim(parsed.Path, "/") != "" {
		return false
	}
	for _, scheme := range schemes {
		if parsed.Scheme == scheme {
			return true
		}
	}
	return false
}
