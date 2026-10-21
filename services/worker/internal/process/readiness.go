package process

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nats-io/nats.go"
)

// checkPublishPermission inspects this same connection's effective principal;
// it does not inject synthetic domain Events into the business Stream.
type requester interface {
	RequestWithContext(context.Context, string, []byte) (*nats.Msg, error)
}

func checkPublishPermission(ctx context.Context, connection requester, user string) error {
	reply, err := connection.RequestWithContext(ctx, "$SYS.REQ.USER.INFO", nil)
	if err != nil {
		return errors.New("worker permission inspection unavailable")
	}
	var info struct {
		Data struct {
			User        string `json:"user"`
			Account     string `json:"account"`
			Permissions struct {
				Publish struct{ Allow, Deny []string } `json:"publish"`
			} `json:"permissions"`
		} `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	if len(reply.Data) > 65536 || json.Unmarshal(reply.Data, &info) != nil || len(info.Error) > 0 ||
		info.Data.User != user || info.Data.Account != "THREADLINE_WORKER" {
		return errors.New("worker permission inspection rejected")
	}
	allow := info.Data.Permissions.Publish.Allow
	deny := info.Data.Permissions.Publish.Deny
	expected := []string{"threadline.domain.events.v1", "$SYS.REQ.USER.INFO", "$JS.API.INFO", "$JS.API.STREAM.NAMES", "$JS.API.STREAM.INFO.THREADLINE_DOMAIN_EVENTS_V1"}
	if !sameSubjects(allow, expected) || !sameSubjects(deny, []string{"_INBOX.threadline.worker.>"}) {
		return errors.New("worker publish permission rejected")
	}
	return nil
}

func sameSubjects(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	remaining := make(map[string]bool, len(expected))
	for _, value := range expected {
		remaining[value] = true
	}
	for _, value := range actual {
		if !remaining[value] {
			return false
		}
		delete(remaining, value)
	}
	return len(remaining) == 0
}
