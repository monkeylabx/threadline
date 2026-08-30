package grantcredential

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"time"
	"unicode/utf8"
)

const transcriptPrefix = "threadline.capability.grant/v1\n"

func canonicalTranscript(grant PresentedGrant) ([]byte, error) {
	if err := validateGrantClaims(grant); err != nil {
		return nil, err
	}

	var output bytes.Buffer
	output.WriteString(transcriptPrefix)
	output.WriteByte('{')
	writeStringField(&output, "capabilities", func() { writeCapabilities(&output, grant) }, true)
	writeStringField(&output, "capabilityGrantId", func() { writeJSONString(&output, grant.CapabilityGrantID) }, false)
	writeStringField(&output, "executionDeviceId", func() { writeJSONString(&output, grant.ExecutionDeviceID) }, false)
	writeStringField(&output, "expiresAt", func() { writeJSONString(&output, formatTimestamp(grant.ExpiresAt)) }, false)
	writeStringField(&output, "grantee", func() { writeActor(&output, grant.Grantee.ID, int32(grant.Grantee.Type)) }, false)
	writeStringField(&output, "initiator", func() { writeActor(&output, grant.Initiator.ID, int32(grant.Initiator.Type)) }, false)
	writeStringField(&output, "issuedAt", func() { writeJSONString(&output, formatTimestamp(grant.IssuedAt)) }, false)
	writeStringField(&output, "nonceHex", func() { writeJSONString(&output, hex.EncodeToString(grant.Nonce)) }, false)
	writeStringField(&output, "policyVersion", func() { writeJSONString(&output, grant.PolicyVersion) }, false)
	writeStringField(&output, "resourceScope", func() { writeScope(&output, grant) }, false)
	writeStringField(&output, "runId", func() { writeJSONString(&output, grant.RunID) }, false)
	writeStringField(&output, "signatureProfile", func() {
		writeJSONString(&output, strconv.FormatInt(int64(grant.SignatureProfile), 10))
	}, false)
	writeStringField(&output, "signedProjectionVersion", func() {
		writeJSONString(&output, strconv.FormatUint(uint64(grant.SignedProjectionVersion), 10))
	}, false)
	writeStringField(&output, "signingKeyId", func() { writeJSONString(&output, grant.SigningKeyID) }, false)
	writeStringField(&output, "taskId", func() { writeJSONString(&output, grant.TaskID) }, false)
	writeStringField(&output, "tenantId", func() { writeJSONString(&output, grant.TenantID) }, false)
	output.WriteByte('}')
	return output.Bytes(), nil
}

func writeStringField(output *bytes.Buffer, name string, value func(), first bool) {
	if !first {
		output.WriteByte(',')
	}
	writeJSONString(output, name)
	output.WriteByte(':')
	value()
}

func writeCapabilities(output *bytes.Buffer, grant PresentedGrant) {
	output.WriteByte('[')
	for index, capability := range grant.Capabilities {
		if index > 0 {
			output.WriteByte(',')
		}
		writeJSONString(output, strconv.FormatInt(int64(capability), 10))
	}
	output.WriteByte(']')
}

func writeActor(output *bytes.Buffer, actorID string, actorType int32) {
	output.WriteByte('{')
	writeStringField(output, "actorId", func() { writeJSONString(output, actorID) }, true)
	writeStringField(output, "actorType", func() {
		writeJSONString(output, strconv.FormatInt(int64(actorType), 10))
	}, false)
	output.WriteByte('}')
}

func writeScope(output *bytes.Buffer, grant PresentedGrant) {
	output.WriteByte('{')
	writeStringArrayField(output, "channelIds", grant.Scope.ChannelIDs, true)
	writeStringArrayField(output, "dmIds", grant.Scope.DMIDs, false)
	writeStringArrayField(output, "eventIds", grant.Scope.EventIDs, false)
	writeStringArrayField(output, "fileIds", grant.Scope.FileIDs, false)
	writeStringArrayField(output, "threadIds", grant.Scope.ThreadIDs, false)
	writeStringArrayField(output, "toolIds", grant.Scope.ToolIDs, false)
	writeStringArrayField(output, "workspaceBindingIds", grant.Scope.WorkspaceBindingIDs, false)
	writeStringArrayField(output, "workspacePathPrefixes", grant.Scope.WorkspacePathPrefixes, false)
	output.WriteByte('}')
}

func writeStringArrayField(output *bytes.Buffer, name string, values []string, first bool) {
	writeStringField(output, name, func() {
		output.WriteByte('[')
		for index, value := range values {
			if index > 0 {
				output.WriteByte(',')
			}
			writeJSONString(output, value)
		}
		output.WriteByte(']')
	}, first)
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
