package codegen_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	localagentv1 "github.com/monkeylabx/threadline/services/gen/threadline/local_agent/v1"
	localagentv1connect "github.com/monkeylabx/threadline/services/gen/threadline/local_agent/v1/local_agentv1connect"
	"google.golang.org/protobuf/proto"
)

func TestLocalAgentWireFixtureRoundtrips(t *testing.T) {
	// test/fixtures/proto/agentd-local/wire.json: SubmitRunInputRequest.
	wire, err := hex.DecodeString("0a0572756e2d611207696e7075742d611a0668c3a96c6c6f")
	if err != nil {
		t.Fatal(err)
	}
	request := new(localagentv1.SubmitRunInputRequest)
	if err := proto.Unmarshal(wire, request); err != nil {
		t.Fatal(err)
	}
	if request.GetRunId() != "run-a" || request.GetInputId() != "input-a" || request.GetText() != "héllo" {
		t.Fatalf("unexpected decoded request: %v", request)
	}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, wire) {
		t.Fatalf("wire changed: %x", encoded)
	}
	if localagentv1connect.LocalAgentServiceName != "threadline.local_agent.v1.LocalAgentService" {
		t.Fatal("unexpected Connect service identity")
	}
}
