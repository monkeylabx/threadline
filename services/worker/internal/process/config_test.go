package process

import (
	"fmt"
	"strings"
	"testing"
)

func configValues() map[string]string {
	return map[string]string{"THREADLINE_WORKER_POSTGRES_DSN": "postgres://worker:secret@localhost/db",
		"THREADLINE_WORKER_NATS_URL": "nats://localhost:4222", "THREADLINE_NATS_WORKER_USER": "worker",
		"THREADLINE_NATS_WORKER_PASSWORD": "password-secret", "THREADLINE_OUTBOX_POLICY_FILE": "policy.json",
		"THREADLINE_WORKER_OWNER_ID": "worker-1"}
}

func TestConfigurationRequiresExplicitDependenciesAndRedactsSecrets(t *testing.T) {
	values := configValues()
	config, err := LoadConfig(func(key string) string { return values[key] })
	if err != nil || config.address != "127.0.0.1:8081" {
		t.Fatal("valid configuration rejected")
	}
	if strings.Contains(fmt.Sprintf("%v %#v", config, config), "secret") {
		t.Fatal("configuration leaked")
	}
	for key := range values {
		changed := configValues()
		delete(changed, key)
		if _, err := LoadConfig(func(key string) string { return changed[key] }); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
	}
}

func TestConfigurationRejectsUnsafeBrokerAndListenInputs(t *testing.T) {
	for _, value := range []string{"http://localhost", "nats://user:secret@localhost", "nats://localhost?x=1", "nats://localhost#fragment", ":"} {
		values := configValues()
		values["THREADLINE_WORKER_NATS_URL"] = value
		if _, err := LoadConfig(func(key string) string { return values[key] }); err == nil {
			t.Fatal("invalid broker accepted")
		}
	}
	values := configValues()
	values["THREADLINE_WORKER_LISTEN_ADDRESS"] = "invalid"
	if _, err := LoadConfig(func(key string) string { return values[key] }); err == nil {
		t.Fatal("invalid address accepted")
	}
	values = configValues()
	values["THREADLINE_WORKER_OWNER_ID"] = " trailing "
	if _, err := LoadConfig(func(key string) string { return values[key] }); err == nil {
		t.Fatal("noncanonical owner accepted")
	}
}
