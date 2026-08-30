package outboxdb

import (
	"crypto/sha256"
	"encoding/base64"
)

const (
	claimTokenRawBytes  = 32
	claimTokenWireBytes = 43
	claimTokenDomain    = "threadline.outbox.claim-token/v1\x00"
)

var strictRawURLBase64 = base64.RawURLEncoding.Strict()

func claimTokenCandidateDigest(wire string) ([sha256.Size]byte, bool) {
	var invalid [sha256.Size]byte
	if len(wire) != claimTokenWireBytes {
		return invalid, false
	}

	raw, err := strictRawURLBase64.DecodeString(wire)
	if err != nil || len(raw) != claimTokenRawBytes || strictRawURLBase64.EncodeToString(raw) != wire {
		return invalid, false
	}
	defer clear(raw)

	hash := sha256.New()
	_, _ = hash.Write([]byte(claimTokenDomain))
	_, _ = hash.Write(raw)
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, true
}
