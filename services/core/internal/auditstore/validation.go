package auditstore

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const identifierMaxBytes = 255

func validCandidate(input candidate) bool {
	if !validIdentifier(input.tenantID) || !validIdentifier(input.auditEventID) ||
		!validActor(input.principal) || !validAction(input.action) ||
		!validOutcome(input.outcome) || !validReason(input.reason) ||
		!validTarget(input.target) || !validIdentifier(input.policyVersion) ||
		!validIdentifier(input.requestID) || !validOptionalIdentifier(input.approvalID) ||
		!validOptionalIdentifier(input.recoveryCaseID) ||
		(input.evidenceDigest != nil && len(input.evidenceDigest) != hashBytes) {
		return false
	}

	switch input.action {
	case actionChannelArchive:
		return input.target.typeID == targetChannel && input.recoveryCaseID == nil
	case actionCapabilityGrantIssue, actionCapabilityGrantRevoke:
		return input.target.typeID == targetCapabilityGrant && input.recoveryCaseID == nil
	case actionRetentionExpire, actionRetentionLegalHoldApply, actionRetentionLegalHoldRelease:
		return input.target.typeID == targetRetentionSubject && input.recoveryCaseID == nil
	case actionRecoveryRequest, actionRecoveryDecision, actionRecoveryCommit:
		return input.target.typeID == targetRecoveryCase && input.recoveryCaseID != nil &&
			*input.recoveryCaseID == input.target.id
	case actionOutboxReplayRequest:
		return input.target.typeID == targetOutboxEntry && input.target.version != nil &&
			input.recoveryCaseID == nil &&
			(input.outcome != outcomeSucceeded || (input.approvalID != nil && input.evidenceDigest != nil))
	default:
		return false
	}
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > identifierMaxBytes || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "*?") {
		return false
	}
	first, _ := utf8.DecodeRuneInString(value)
	last, _ := utf8.DecodeLastRuneInString(value)
	if isECMAScriptTrimSpace(first) || isECMAScriptTrimSpace(last) {
		return false
	}
	return !strings.ContainsFunc(value, unicode.IsControl)
}

func isECMAScriptTrimSpace(value rune) bool {
	switch value {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680',
		'\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
		return true
	default:
		return value >= '\u2000' && value <= '\u200a'
	}
}

func validOptionalIdentifier(value *string) bool {
	return value == nil || validIdentifier(*value)
}

func validActor(value actor) bool {
	return value.typeID >= actorHuman && value.typeID <= actorService && validIdentifier(value.id)
}

func validTarget(value target) bool {
	if !validTargetType(value.typeID) || !validIdentifier(value.id) {
		return false
	}
	return value.version == nil || *value.version > 0
}

func validAction(value action) bool {
	switch value {
	case actionChannelArchive, actionCapabilityGrantIssue, actionCapabilityGrantRevoke,
		actionRetentionExpire, actionRetentionLegalHoldApply, actionRetentionLegalHoldRelease,
		actionRecoveryRequest, actionRecoveryDecision, actionRecoveryCommit, actionOutboxReplayRequest:
		return true
	default:
		return false
	}
}

func validOutcome(value outcome) bool {
	return value == outcomeSucceeded || value == outcomeDenied || value == outcomeFailed
}

func validReason(value reason) bool {
	switch value {
	case reasonAuthorized, reasonAuthorizationDenied, reasonEvidenceInvalid, reasonPolicyDenied,
		reasonRetentionExpired, reasonStateConflict, reasonInvalidInput, reasonInternalFailure:
		return true
	default:
		return false
	}
}

func validTargetType(value targetType) bool {
	switch value {
	case targetChannel, targetCapabilityGrant, targetRetentionSubject, targetRecoveryCase, targetOutboxEntry:
		return true
	default:
		return false
	}
}
