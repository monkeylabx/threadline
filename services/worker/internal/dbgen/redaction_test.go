package dbgen

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestTokenBearingValuesAreRedactedFromFormattingAndJSON(t *testing.T) {
	t.Parallel()

	secret := []byte("outbox-secret-marker")
	values := []any{
		ClaimTransactionalOutboxBatchRow{RawClaimToken: secret, Payload: secret},
		RenewTransactionalOutboxClaimParams{CandidateDigest: secret},
		AcknowledgeTransactionalOutboxPublishedParams{CandidateDigest: secret},
		RecordTransactionalOutboxPublishFailureParams{CandidateDigest: secret},
	}
	for _, value := range values {
		value := value
		t.Run(fmt.Sprintf("%T", value), func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			for _, rendered := range []string{
				fmt.Sprint(value),
				fmt.Sprintf("%+v", value),
				fmt.Sprintf("%#v", value),
				string(encoded),
			} {
				if strings.Contains(rendered, string(secret)) || !strings.Contains(rendered, redactedOutboxAuthority) {
					t.Fatalf("token-bearing value was not safely redacted: %q", rendered)
				}
			}
		})
	}
}
