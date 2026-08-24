package auditstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
	"unicode/utf8"
)

const transcriptPrefix = "threadline.audit.event/v1\n"

func hashEvent(input candidate, slot appendSlot) ([]byte, []byte, error) {
	if !validCandidate(input) || !validSlot(input.tenantID, slot) {
		return nil, nil, storeError(errorInvalidInput)
	}
	transcript := canonicalTranscript(input, slot)
	digest := sha256.Sum256(transcript)
	return transcript, digest[:], nil
}

func validSlot(tenantID string, slot appendSlot) bool {
	return slot.tenantID == tenantID && slot.tenantSequence > 0 && !slot.recordedAt.IsZero() &&
		slot.recordedAt.Location() != nil && slot.transactionID > 0 &&
		len(slot.previousEventHash) == hashBytes &&
		((slot.tenantSequence == 1 && slot.previousAuditEventID == nil && bytes.Equal(slot.previousEventHash, make([]byte, hashBytes))) ||
			(slot.tenantSequence > 1 && slot.previousAuditEventID != nil && validIdentifier(*slot.previousAuditEventID)))
}

func canonicalTranscript(input candidate, slot appendSlot) []byte {
	var output bytes.Buffer
	output.WriteString(transcriptPrefix)
	output.WriteByte('{')
	writeField(&output, "action", func() { writeJSONString(&output, string(input.action)) }, true)
	writeField(&output, "approvalId", func() { writeNullableString(&output, input.approvalID) }, false)
	writeField(&output, "auditEventId", func() { writeJSONString(&output, input.auditEventID) }, false)
	writeField(&output, "contractVersion", func() { writeJSONString(&output, "1") }, false)
	writeField(&output, "evidenceDigestHex", func() { writeNullableHex(&output, input.evidenceDigest) }, false)
	writeField(&output, "outcome", func() { writeJSONString(&output, string(input.outcome)) }, false)
	writeField(&output, "policyVersion", func() { writeJSONString(&output, input.policyVersion) }, false)
	writeField(&output, "previousEventHashHex", func() { writeJSONString(&output, hex.EncodeToString(slot.previousEventHash)) }, false)
	writeField(&output, "principal", func() { writeActor(&output, input.principal) }, false)
	writeField(&output, "reason", func() { writeJSONString(&output, string(input.reason)) }, false)
	writeField(&output, "recordedAt", func() { writeJSONString(&output, formatTimestamp(slot.recordedAt)) }, false)
	writeField(&output, "recoveryCaseId", func() { writeNullableString(&output, input.recoveryCaseID) }, false)
	writeField(&output, "requestId", func() { writeJSONString(&output, input.requestID) }, false)
	writeField(&output, "target", func() { writeTarget(&output, input.target) }, false)
	writeField(&output, "tenantId", func() { writeJSONString(&output, input.tenantID) }, false)
	writeField(&output, "tenantSequence", func() {
		writeJSONString(&output, strconv.FormatInt(slot.tenantSequence, 10))
	}, false)
	output.WriteByte('}')
	return output.Bytes()
}

func writeField(output *bytes.Buffer, name string, value func(), first bool) {
	if !first {
		output.WriteByte(',')
	}
	writeJSONString(output, name)
	output.WriteByte(':')
	value()
}

func writeActor(output *bytes.Buffer, value actor) {
	output.WriteByte('{')
	writeField(output, "actorId", func() { writeJSONString(output, value.id) }, true)
	writeField(output, "actorType", func() {
		writeJSONString(output, strconv.FormatInt(int64(value.typeID), 10))
	}, false)
	output.WriteByte('}')
}

func writeTarget(output *bytes.Buffer, value target) {
	output.WriteByte('{')
	writeField(output, "targetId", func() { writeJSONString(output, value.id) }, true)
	writeField(output, "targetType", func() { writeJSONString(output, string(value.typeID)) }, false)
	writeField(output, "targetVersion", func() {
		if value.version == nil {
			output.WriteString("null")
			return
		}
		writeJSONString(output, strconv.FormatInt(*value.version, 10))
	}, false)
	output.WriteByte('}')
}

func writeNullableString(output *bytes.Buffer, value *string) {
	if value == nil {
		output.WriteString("null")
		return
	}
	writeJSONString(output, *value)
}

func writeNullableHex(output *bytes.Buffer, value []byte) {
	if value == nil {
		output.WriteString("null")
		return
	}
	writeJSONString(output, hex.EncodeToString(value))
}

func writeJSONString(output *bytes.Buffer, value string) {
	const hexDigits = "0123456789abcdef"
	output.WriteByte('"')
	for offset := 0; offset < len(value); {
		character, size := utf8.DecodeRuneInString(value[offset:])
		switch character {
		case '"', '\\':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '\b':
			output.WriteString("\\b")
		case '\t':
			output.WriteString("\\t")
		case '\n':
			output.WriteString("\\n")
		case '\f':
			output.WriteString("\\f")
		case '\r':
			output.WriteString("\\r")
		default:
			if character < 0x20 {
				output.WriteString("\\u00")
				output.WriteByte(hexDigits[byte(character)>>4])
				output.WriteByte(hexDigits[byte(character)&0x0f])
			} else {
				output.WriteString(value[offset : offset+size])
			}
		}
		offset += size
	}
	output.WriteByte('"')
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func storeError(code errorCode) error { return &storeFailure{code: code} }
