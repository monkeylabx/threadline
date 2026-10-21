package process

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
)

type permissionReply struct {
	body []byte
	err  error
}

func (r permissionReply) RequestWithContext(context.Context, string, []byte) (*nats.Msg, error) {
	return &nats.Msg{Data: r.body}, r.err
}

func permissionBody() map[string]any {
	return map[string]any{"data": map[string]any{"user": "worker", "account": "THREADLINE_WORKER",
		"permissions": map[string]any{"publish": map[string]any{
			"allow": []string{"threadline.domain.events.v1", "$SYS.REQ.USER.INFO", "$JS.API.INFO", "$JS.API.STREAM.NAMES", "$JS.API.STREAM.INFO.THREADLINE_DOMAIN_EVENTS_V1"},
			"deny":  []string{"_INBOX.threadline.worker.>"}}}}}
}

func TestPermissionInspectionRequiresExactEffectiveAuthority(t *testing.T) {
	body, err := json.Marshal(permissionBody())
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPublishPermission(context.Background(), permissionReply{body: body}, "worker"); err != nil {
		t.Fatal(err)
	}
	changes := []func(map[string]any){
		func(m map[string]any) { m["error"] = map[string]any{"code": 403} },
		func(m map[string]any) { m["data"].(map[string]any)["user"] = "foreign" },
		func(m map[string]any) { m["data"].(map[string]any)["account"] = "SYS" },
		func(m map[string]any) { m["data"].(map[string]any)["permissions"] = map[string]any{} },
	}
	for _, change := range changes {
		m := permissionBody()
		change(m)
		body, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkPublishPermission(context.Background(), permissionReply{body: body}, "worker"); err == nil {
			t.Fatal("invalid authority accepted")
		}
	}
	for _, reply := range []permissionReply{{err: errors.New("secret")}, {body: []byte("invalid")}, {body: make([]byte, 65537)}} {
		if err := checkPublishPermission(context.Background(), reply, "worker"); err == nil {
			t.Fatal("bad response accepted")
		}
	}
}
