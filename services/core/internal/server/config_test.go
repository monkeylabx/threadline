package server

import (
	"fmt"
	"strings"
	"testing"
)

const syntheticDSN = "postgres://threadline:secret-pw-synthetic@127.0.0.1:5432/threadline?sslmode=disable"

func lookupFrom(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func TestLoadConfigDefaultsListenAddress(t *testing.T) {
	t.Parallel()
	config, err := LoadConfig(lookupFrom(map[string]string{EnvPostgresDSN: syntheticDSN}))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.ListenAddress() != defaultListenAddress || config.PostgresDSN() != syntheticDSN {
		t.Fatalf("LoadConfig() = %q listen, want default and the given DSN", config.ListenAddress())
	}
}

func TestLoadConfigUsesExplicitListenAddress(t *testing.T) {
	t.Parallel()
	config, err := LoadConfig(lookupFrom(map[string]string{
		EnvPostgresDSN: syntheticDSN, EnvListenAddress: "127.0.0.1:0",
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.ListenAddress() != "127.0.0.1:0" {
		t.Fatalf("ListenAddress() = %q, want 127.0.0.1:0", config.ListenAddress())
	}
}

func TestLoadConfigRejectsInvalidInputWithoutEchoingSecrets(t *testing.T) {
	t.Parallel()
	cases := map[string]func(string) (string, bool){
		"nil lookup":      nil,
		"missing dsn":     lookupFrom(map[string]string{}),
		"empty dsn":       lookupFrom(map[string]string{EnvPostgresDSN: ""}),
		"malformed dsn":   lookupFrom(map[string]string{EnvPostgresDSN: "not a dsn secret-pw-synthetic"}),
		"bad listen addr": lookupFrom(map[string]string{EnvPostgresDSN: syntheticDSN, EnvListenAddress: "nohostport"}),
	}
	for name, lookup := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadConfig(lookup)
			if err == nil {
				t.Fatal("LoadConfig() error = nil, want rejection")
			}
			if strings.Contains(err.Error(), "secret-pw-synthetic") {
				t.Fatal("LoadConfig() error echoed the DSN")
			}
		})
	}
}

func TestConfigFormattingRedactsDSN(t *testing.T) {
	t.Parallel()
	config, err := LoadConfig(lookupFrom(map[string]string{EnvPostgresDSN: syntheticDSN}))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	for _, rendered := range []string{fmt.Sprint(config), fmt.Sprintf("%#v", config), fmt.Sprintf("%+v", config)} {
		if strings.Contains(rendered, "secret-pw-synthetic") {
			t.Fatalf("formatted config %q leaked the DSN", rendered)
		}
	}
}
