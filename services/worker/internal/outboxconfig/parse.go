package outboxconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"strconv"
	"time"
)

const (
	maximumDocumentBytes = 4096
	requiredKeyCount     = 15
	payloadHardBytes     = 262_144
	wireHardBytes        = 327_680
	duplicateWindowMS    = 120_000
)

type policyValues struct {
	payloadHardBytes   uint32
	wireHardBytes      uint32
	batchSize          uint32
	leaseMS            uint32
	absoluteLifetimeMS uint32
	eventRetryCeiling  uint32
	transportBaseMS    uint32
	transportCapMS     uint32
	unknownBaseMS      uint32
	unknownCapMS       uint32
	eventBaseMS        uint32
	eventCapMS         uint32
	retentionDays      uint32
	duplicateWindowMS  uint32
}

// Parse accepts exactly one bounded JSON object and returns a detached policy
// snapshot. It never retains or mutates document.
func Parse(document []byte) (Snapshot, error) {
	if len(document) == 0 || len(document) > maximumDocumentBytes ||
		bytes.HasPrefix(document, []byte{0xef, 0xbb, 0xbf}) {
		return Snapshot{}, invalidInput()
	}

	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return Snapshot{}, invalidInput()
	}

	seen := make(map[string]struct{}, requiredKeyCount)
	values := policyValues{}
	policyID := ""
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, ok := keyToken.(string)
		if tokenErr != nil || !ok {
			return Snapshot{}, invalidInput()
		}
		if _, duplicate := seen[key]; duplicate {
			return Snapshot{}, invalidInput()
		}
		seen[key] = struct{}{}

		valueToken, tokenErr := decoder.Token()
		if tokenErr != nil {
			return Snapshot{}, invalidInput()
		}
		if key == "policy_id" {
			value, stringValue := valueToken.(string)
			if !stringValue {
				return Snapshot{}, invalidInput()
			}
			policyID = value
			continue
		}
		number, numberValue := valueToken.(json.Number)
		if !numberValue {
			return Snapshot{}, invalidInput()
		}
		parsed, parseErr := parseUint32(string(number))
		if parseErr != nil || !values.set(key, parsed) {
			return Snapshot{}, invalidInput()
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || len(seen) != requiredKeyCount {
		return Snapshot{}, invalidInput()
	}
	if _, err = decoder.Token(); err != io.EOF {
		return Snapshot{}, invalidInput()
	}
	if policyID != PolicyV1ID || !values.valid() {
		return Snapshot{}, invalidInput()
	}
	return newSnapshot(policyID, values), nil
}

func (values *policyValues) set(key string, value uint32) bool {
	switch key {
	case "payload_hard_bytes":
		values.payloadHardBytes = value
	case "wire_hard_bytes":
		values.wireHardBytes = value
	case "batch_size":
		values.batchSize = value
	case "lease_ms":
		values.leaseMS = value
	case "absolute_lifetime_ms":
		values.absoluteLifetimeMS = value
	case "event_retry_ceiling":
		values.eventRetryCeiling = value
	case "transport_base_ms":
		values.transportBaseMS = value
	case "transport_cap_ms":
		values.transportCapMS = value
	case "unknown_base_ms":
		values.unknownBaseMS = value
	case "unknown_cap_ms":
		values.unknownCapMS = value
	case "event_base_ms":
		values.eventBaseMS = value
	case "event_cap_ms":
		values.eventCapMS = value
	case "retention_days":
		values.retentionDays = value
	case "duplicate_window_ms":
		values.duplicateWindowMS = value
	default:
		return false
	}
	return true
}

func parseUint32(token string) (uint32, error) {
	if token == "" || (len(token) > 1 && token[0] == '0') {
		return 0, strconv.ErrSyntax
	}
	for _, character := range []byte(token) {
		if character < '0' || character > '9' {
			return 0, strconv.ErrSyntax
		}
	}
	parsed, err := strconv.ParseUint(token, 10, 32)
	return uint32(parsed), err
}

func (values policyValues) valid() bool {
	return values.payloadHardBytes == payloadHardBytes &&
		values.wireHardBytes == wireHardBytes &&
		between(values.batchSize, 1, 256) &&
		between(values.leaseMS, 5_000, 120_000) &&
		between(values.absoluteLifetimeMS, 30_000, 900_000) &&
		values.absoluteLifetimeMS >= 2*values.leaseMS &&
		between(values.eventRetryCeiling, 1, 20) &&
		validBackoff(values.transportBaseMS, values.transportCapMS, 100, 10_000, 1_000, 300_000) &&
		validBackoff(values.unknownBaseMS, values.unknownCapMS, 500, 30_000, 5_000, 900_000) &&
		validBackoff(values.eventBaseMS, values.eventCapMS, 500, 30_000, 5_000, 900_000) &&
		between(values.retentionDays, 30, 365) &&
		values.duplicateWindowMS == duplicateWindowMS
}

func between(value, minimum, maximum uint32) bool {
	return value >= minimum && value <= maximum
}

func validBackoff(base, cap, minimumBase, maximumBase, minimumCap, maximumCap uint32) bool {
	return between(base, minimumBase, maximumBase) && between(cap, minimumCap, maximumCap) && cap >= base
}

func newSnapshot(policyID string, values policyValues) Snapshot {
	preimage := policyPreimage(values)
	return Snapshot{
		policyID:        policyID,
		payloadBytes:    values.payloadHardBytes,
		wireBytes:       values.wireHardBytes,
		batchSize:       values.batchSize,
		lease:           time.Duration(values.leaseMS) * time.Millisecond,
		absolute:        time.Duration(values.absoluteLifetimeMS) * time.Millisecond,
		eventCeiling:    values.eventRetryCeiling,
		transport:       backoff(values.transportBaseMS, values.transportCapMS),
		unknown:         backoff(values.unknownBaseMS, values.unknownCapMS),
		event:           backoff(values.eventBaseMS, values.eventCapMS),
		retentionDays:   values.retentionDays,
		duplicateWindow: time.Duration(values.duplicateWindowMS) * time.Millisecond,
		digest:          sha256.Sum256(preimage),
	}
}

func backoff(base, cap uint32) Backoff {
	return Backoff{base: time.Duration(base) * time.Millisecond, cap: time.Duration(cap) * time.Millisecond}
}

func policyPreimage(values policyValues) []byte {
	const prefix = "threadline.outbox.policy-snapshot/v1\x00"
	ordered := [...]uint32{
		values.payloadHardBytes, values.wireHardBytes, values.batchSize,
		values.leaseMS, values.absoluteLifetimeMS, values.eventRetryCeiling,
		values.transportBaseMS, values.transportCapMS, values.unknownBaseMS,
		values.unknownCapMS, values.eventBaseMS, values.eventCapMS,
		values.retentionDays, values.duplicateWindowMS,
	}
	preimage := make([]byte, len(prefix)+4*len(ordered))
	copy(preimage, prefix)
	offset := len(prefix)
	for _, value := range ordered {
		binary.BigEndian.PutUint32(preimage[offset:offset+4], value)
		offset += 4
	}
	return preimage
}
