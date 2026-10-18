package devsession

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	maximumDocumentBytes = 65_536
	maximumFixtures      = 64
	minimumTokenBytes    = 16
	maximumFieldBytes    = 128
)

// Fixture is one static dev session. ExpiresAt is optional; Revoked marks a
// session that must answer VerificationRevoked so clients can be tested
// against revocation without a real Session service.
type Fixture struct {
	Token     string    `json:"token"`
	TenantID  string    `json:"tenant_id"`
	ActorType string    `json:"actor_type"`
	ActorID   string    `json:"actor_id"`
	DeviceID  string    `json:"device_id"`
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

type document struct {
	Sessions []Fixture `json:"sessions"`
}

// Parse decodes a fixture document: a JSON object with a "sessions" array,
// unknown fields rejected, at most 64 entries, every identifier non-blank and
// at most 128 bytes, tokens at least 16 bytes and unique. It never logs or
// echoes a token.
func Parse(input []byte) ([]Fixture, error) {
	if len(input) == 0 || len(input) > maximumDocumentBytes {
		return nil, errors.New("devsession: fixture document size invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var parsed document
	if err := decoder.Decode(&parsed); err != nil {
		return nil, errors.New("devsession: fixture document malformed")
	}
	if decoder.More() {
		return nil, errors.New("devsession: fixture document has trailing content")
	}
	if len(parsed.Sessions) == 0 || len(parsed.Sessions) > maximumFixtures {
		return nil, errors.New("devsession: fixture count invalid")
	}
	seen := make(map[string]struct{}, len(parsed.Sessions))
	for _, fixture := range parsed.Sessions {
		if !validFixture(fixture) {
			return nil, errors.New("devsession: fixture invalid")
		}
		if _, duplicate := seen[fixture.Token]; duplicate {
			return nil, errors.New("devsession: duplicate fixture token")
		}
		seen[fixture.Token] = struct{}{}
	}
	return parsed.Sessions, nil
}

func validFixture(fixture Fixture) bool {
	_, knownActor := actorTypes[fixture.ActorType]
	return knownActor &&
		len(fixture.Token) >= minimumTokenBytes && validIdentifier(fixture.Token) &&
		validIdentifier(fixture.TenantID) && validIdentifier(fixture.ActorID) &&
		validIdentifier(fixture.DeviceID) && validIdentifier(fixture.SessionID)
}

var actorTypes = map[string]rpcmiddleware.ActorType{
	"human":   rpcmiddleware.ActorTypeHuman,
	"agent":   rpcmiddleware.ActorTypeAgent,
	"service": rpcmiddleware.ActorTypeService,
}

func validIdentifier(value string) bool {
	return value != "" && len(value) <= maximumFieldBytes && value == strings.TrimSpace(value) &&
		!strings.ContainsFunc(value, unicode.IsControl)
}
