package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRealtimeConfigurationIsExplicitAndRedacted(t *testing.T) {
	values := map[string]string{"THREADLINE_REALTIME_CORE_URL": "http://127.0.0.1:8080", "THREADLINE_REALTIME_NATS_URL": "nats://127.0.0.1:4222", "THREADLINE_NATS_REALTIME_USER": "reader", "THREADLINE_NATS_REALTIME_PASSWORD": "secret"}
	c, err := load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%v %#v", c, c), "secret") {
		t.Fatal("credential leaked")
	}
	for key := range values {
		original := values[key]
		values[key] = ""
		if _, err := load(func(key string) string { return values[key] }); err == nil {
			t.Fatal("missing config accepted")
		}
		values[key] = original
	}
	for _, value := range []string{"http://user:secret@host", "http://host/path", "http://host?secret=x", "http://host#x", ":", ""} {
		if endpoint(value, "http") {
			t.Fatal("unsafe endpoint accepted")
		}
	}
}

func TestLiveRealtimeProcessStartsAndStops(t *testing.T) {
	broker := os.Getenv("THREADLINE_TEST_REALTIME_NATS_URL")
	if broker == "" {
		t.Skip("real NATS fixture not configured")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := config{address: "127.0.0.1:0", core: "http://127.0.0.1:8080", broker: broker, user: "threadline_realtime_dev", password: "threadline-worker-dev-only"}
	if err := runWithContext(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := serve(ctx, "invalid", nil); err == nil {
		t.Fatal("invalid listen address accepted")
	}
}
