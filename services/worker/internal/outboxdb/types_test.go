package outboxdb

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestClaimTokenCreatedFromDatabaseBytesIsCanonicalAndRedacted(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	token, ok := newClaimToken(raw)
	if !ok {
		t.Fatal("newClaimToken rejected 32 bytes")
	}
	if got := token.wire; got != goldenClaimTokenWire {
		t.Fatalf("canonical wire = %q, want Golden", got)
	}
	for _, rendered := range []string{
		fmt.Sprint(token),
		fmt.Sprintf("%+v", token),
		fmt.Sprintf("%#v", token),
		fmt.Sprintf("%+v", claimFence{token: token}),
		fmt.Sprintf("%#v", claimFence{token: token}),
	} {
		if strings.Contains(rendered, goldenClaimTokenWire) || strings.Contains(rendered, "0 1 2 3") {
			t.Fatalf("token rendering exposed secret: %q", rendered)
		}
	}
	if _, ok := newClaimToken(raw[:31]); ok {
		t.Fatal("newClaimToken accepted 31 bytes")
	}
}

func TestStableFailureRenderingNeverIncludesOperationFacts(t *testing.T) {
	t.Parallel()

	err := operationError(errorClaimDenied)
	if got, want := err.Error(), "transactional outbox: claim-denied"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if got := err.category(); got != errorClaimDenied {
		t.Fatalf("category = %q, want %q", got, errorClaimDenied)
	}
}

func TestTrustedInputsAreBoundedBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	validClaim := claimRequest{claimOwnerID: "worker-1", batchSize: 64}
	for name, request := range map[string]claimRequest{
		"empty owner":       {claimOwnerID: "", batchSize: 64},
		"uncanonical owner": {claimOwnerID: " worker-1", batchSize: 64},
		"control in owner":  {claimOwnerID: "worker\n1", batchSize: 64},
		"owner too long":    {claimOwnerID: strings.Repeat("é", 65), batchSize: 64},
		"zero batch":        {claimOwnerID: "worker-1", batchSize: 0},
		"large batch":       {claimOwnerID: "worker-1", batchSize: 257},
	} {
		if validClaimRequest(request) {
			t.Errorf("%s accepted: %#v", name, request)
		}
	}
	if !validClaimRequest(validClaim) {
		t.Fatal("valid claim request rejected")
	}

	validAck := pubAck{stream: "DOMAIN_EVENTS", sequence: 1, messageID: strings.Repeat("a", 64)}
	for name, ack := range map[string]pubAck{
		"empty stream":       {sequence: 1, messageID: strings.Repeat("a", 64)},
		"uncanonical stream": {stream: " DOMAIN_EVENTS", sequence: 1, messageID: strings.Repeat("a", 64)},
		"stream too long":    {stream: strings.Repeat("s", 256), sequence: 1, messageID: strings.Repeat("a", 64)},
		"zero sequence":      {stream: "DOMAIN_EVENTS", messageID: strings.Repeat("a", 64)},
		"uppercase message":  {stream: "DOMAIN_EVENTS", sequence: 1, messageID: strings.Repeat("A", 64)},
		"short message":      {stream: "DOMAIN_EVENTS", sequence: 1, messageID: strings.Repeat("a", 63)},
	} {
		if validPubAck(ack) {
			t.Errorf("%s accepted: %#v", name, ack)
		}
	}
	if !validPubAck(validAck) {
		t.Fatal("valid PubAck rejected")
	}
}

func TestPublishFailureCodeAllowlist(t *testing.T) {
	t.Parallel()

	for _, code := range []failureCode{
		failureTransportUnavailable,
		failurePublishOutcomeUnknown,
		failureEventRetryable,
		failureEventPermanent,
	} {
		if !validFailureCode(code) {
			t.Errorf("known code %q rejected", code)
		}
	}
	if validFailureCode("future-code") {
		t.Fatal("unknown failure code accepted")
	}
}
