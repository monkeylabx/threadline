package grantpolicy

import (
	"reflect"
	"testing"
	"time"
)

func TestValidateAndNarrowReturnsBoundCanonicalRequest(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.August, 24, 1, 0, 0, 0, time.FixedZone("test", 8*60*60))
	expiresAt := at.Add(15 * time.Minute)
	request := Request{
		Capabilities: []Capability{CapabilityFileRead, CapabilityMessageRead},
		Scope: ResourceScope{
			ChannelIDs: []string{"channel-b", "channel-a"},
			FileIDs:    []string{"file-a"},
		},
		ExpiresAt: expiresAt,
	}
	authority := IssuanceAuthority{
		TenantID:      "tenant-a",
		TaskID:        "task-a",
		RunID:         "run-a",
		DeviceID:      "device-a",
		Grantee:       ActorRef{Type: ActorTypeAgent, ID: "agent-a"},
		Initiator:     ActorRef{Type: ActorTypeHuman, ID: "human-a"},
		PolicyVersion: "policy-v1",
		Capabilities: []Capability{
			CapabilityMessageRead,
			CapabilityFileRead,
			CapabilityToolInvoke,
		},
		Scope: ResourceScope{
			ChannelIDs: []string{"channel-a", "channel-b", "channel-c"},
			FileIDs:    []string{"file-a", "file-b"},
			ToolIDs:    []string{"tool-a"},
		},
		NotAfter: at.Add(time.Hour),
	}

	got, err := ValidateAndNarrow(at, request, authority)
	if err != nil {
		t.Fatalf("ValidateAndNarrow() error = %v", err)
	}
	if got.TenantID() != authority.TenantID || got.TaskID() != authority.TaskID ||
		got.RunID() != authority.RunID || got.DeviceID() != authority.DeviceID ||
		got.Grantee() != authority.Grantee || got.Initiator() != authority.Initiator ||
		got.PolicyVersion() != authority.PolicyVersion {
		t.Fatalf("validated request lost trusted bindings: %#v", got)
	}
	if !got.ExpiresAt().Equal(expiresAt) {
		t.Fatalf("ExpiresAt() = %v, want %v", got.ExpiresAt(), expiresAt)
	}
	if want := []Capability{CapabilityMessageRead, CapabilityFileRead}; !reflect.DeepEqual(got.Capabilities(), want) {
		t.Fatalf("Capabilities() = %v, want %v", got.Capabilities(), want)
	}
	if want := []string{"channel-a", "channel-b"}; !reflect.DeepEqual(got.Scope().ChannelIDs, want) {
		t.Fatalf("Scope().ChannelIDs = %v, want %v", got.Scope().ChannelIDs, want)
	}
}
