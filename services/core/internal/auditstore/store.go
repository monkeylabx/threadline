package auditstore

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
)

// appendEvent appends one candidate inside tx. The caller owns transaction
// lifetime and must roll back after every returned error.
func appendEvent(ctx context.Context, tx pgx.Tx, input candidate) (event, error) {
	if err := ctx.Err(); err != nil {
		return event{}, err
	}
	if tx == nil || !validCandidate(input) {
		return event{}, storeError(errorInvalidInput)
	}

	queries := dbgen.New(tx)
	var isolation string
	if err := tx.QueryRow(ctx, "SELECT current_setting('transaction_isolation')::text").Scan(&isolation); err != nil || isolation != "read committed" {
		return event{}, persistenceError(ctx)
	}

	if observed, found, observeErr := observe(ctx, queries, input); observeErr != nil {
		return event{}, observeErr
	} else if found {
		return observed, nil
	}

	if err := queries.EnsureAuditTenantHead(ctx, input.tenantID); err != nil {
		return event{}, persistenceError(ctx)
	}
	slotRow, err := queries.LockAuditAppendSlot(ctx, input.tenantID)
	if err != nil {
		return event{}, persistenceError(ctx)
	}
	if observed, found, observeErr := observe(ctx, queries, input); observeErr != nil {
		return event{}, observeErr
	} else if found {
		return observed, nil
	}

	slot := slotFromRow(slotRow)
	_, digest, err := hashEvent(input, slot)
	if err != nil {
		return event{}, err
	}
	inserted, err := queries.AppendAuditEventAndAdvanceHead(ctx, appendParams(input, slotRow, digest))
	if err != nil {
		return event{}, persistenceError(ctx)
	}
	result, ok := eventFromInserted(inserted, dispositionCreated)
	if !ok || !sameCandidate(result.candidate, input) || result.tenantSequence != slot.tenantSequence ||
		!result.recordedAt.Equal(slot.recordedAt.UTC()) ||
		!bytes.Equal(result.previousEventHash, slot.previousEventHash) || !bytes.Equal(result.eventHash, digest) {
		return event{}, storeError(errorPersistence)
	}
	return result, nil
}

func observe(ctx context.Context, queries *dbgen.Queries, input candidate) (event, bool, error) {
	row, err := queries.ObserveAuditEventIdempotency(ctx, observationParams(input))
	if errors.Is(err, pgx.ErrNoRows) {
		return event{}, false, nil
	}
	if err != nil {
		return event{}, false, persistenceError(ctx)
	}
	if !row.ExactMatch {
		return event{}, false, storeError(errorIdempotencyConflict)
	}
	result, ok := eventFromObserved(row)
	if !ok || !sameCandidate(result.candidate, input) {
		return event{}, false, storeError(errorPersistence)
	}
	_, digest, hashErr := hashStoredEvent(result)
	if hashErr != nil || !bytes.Equal(digest, result.eventHash) {
		return event{}, false, storeError(errorPersistence)
	}
	return result, true, nil
}

func hashStoredEvent(value event) ([]byte, []byte, error) {
	previousID := "stored-predecessor"
	slot := appendSlot{
		tenantID: value.tenantID, tenantSequence: value.tenantSequence,
		recordedAt: value.recordedAt, transactionID: 1,
		previousEventHash: bytes.Clone(value.previousEventHash),
	}
	if value.tenantSequence > 1 {
		slot.previousAuditEventID = &previousID
	}
	return hashEvent(value.candidate, slot)
}

func slotFromRow(row dbgen.LockAuditAppendSlotRow) appendSlot {
	return appendSlot{
		tenantID: row.TenantID, tenantSequence: row.TenantSequence,
		recordedAt: row.RecordedAt.Time.UTC(), transactionID: row.SlotTransactionID,
		previousAuditEventID: cloneString(row.PreviousAuditEventID),
		previousEventHash:    bytes.Clone(row.PreviousEventHash),
	}
}

func appendParams(input candidate, slot dbgen.LockAuditAppendSlotRow, digest []byte) dbgen.AppendAuditEventAndAdvanceHeadParams {
	return dbgen.AppendAuditEventAndAdvanceHeadParams{
		SlotTransactionID:    slot.SlotTransactionID,
		RecordedAt:           slot.RecordedAt,
		TenantID:             input.tenantID,
		AuditEventID:         input.auditEventID,
		TenantSequence:       slot.TenantSequence,
		PrincipalActorType:   int16(input.principal.typeID),
		PrincipalActorID:     input.principal.id,
		Action:               string(input.action),
		Outcome:              string(input.outcome),
		Reason:               string(input.reason),
		TargetType:           string(input.target.typeID),
		TargetID:             input.target.id,
		TargetVersion:        cloneInt64(input.target.version),
		PolicyVersion:        input.policyVersion,
		RequestID:            input.requestID,
		ApprovalID:           cloneString(input.approvalID),
		RecoveryCaseID:       cloneString(input.recoveryCaseID),
		EvidenceDigest:       bytes.Clone(input.evidenceDigest),
		PreviousEventHash:    bytes.Clone(slot.PreviousEventHash),
		EventHash:            bytes.Clone(digest),
		PreviousAuditEventID: cloneString(slot.PreviousAuditEventID),
	}
}

func observationParams(input candidate) dbgen.ObserveAuditEventIdempotencyParams {
	return dbgen.ObserveAuditEventIdempotencyParams{
		TenantID: input.tenantID, AuditEventID: input.auditEventID,
		PrincipalActorType: int16(input.principal.typeID), PrincipalActorID: input.principal.id,
		Action: string(input.action), Outcome: string(input.outcome), Reason: string(input.reason),
		TargetType: string(input.target.typeID), TargetID: input.target.id,
		TargetVersion: cloneInt64(input.target.version), PolicyVersion: input.policyVersion,
		RequestID: input.requestID, ApprovalID: cloneString(input.approvalID),
		RecoveryCaseID: cloneString(input.recoveryCaseID), EvidenceDigest: bytes.Clone(input.evidenceDigest),
	}
}

func eventFromInserted(row dbgen.AppendAuditEventAndAdvanceHeadRow, state disposition) (event, bool) {
	return eventFromDatabase(
		row.TenantID, row.AuditEventID, row.ContractVersion, row.TenantSequence, row.RecordedAt,
		row.PrincipalActorType, row.PrincipalActorID, row.Action, row.Outcome, row.Reason,
		row.TargetType, row.TargetID, row.TargetVersion, row.PolicyVersion, row.RequestID,
		row.ApprovalID, row.RecoveryCaseID, row.EvidenceDigest, row.PreviousEventHash, row.EventHash, state,
	)
}

func eventFromObserved(row dbgen.ObserveAuditEventIdempotencyRow) (event, bool) {
	return eventFromDatabase(
		row.TenantID, row.AuditEventID, row.ContractVersion, row.TenantSequence, row.RecordedAt,
		row.PrincipalActorType, row.PrincipalActorID, row.Action, row.Outcome, row.Reason,
		row.TargetType, row.TargetID, row.TargetVersion, row.PolicyVersion, row.RequestID,
		row.ApprovalID, row.RecoveryCaseID, row.EvidenceDigest, row.PreviousEventHash, row.EventHash,
		dispositionAlreadyPresent,
	)
}

func eventFromDatabase(
	tenantID, eventID string,
	contractVersion int16,
	sequence int64,
	recordedAt pgtype.Timestamptz,
	principalType int16,
	actorID, eventAction, eventOutcome, eventReason, eventTargetType, targetID string,
	targetVersion *int64,
	policyVersion, requestID string,
	approvalID, recoveryCaseID *string,
	evidenceDigest, previousHash, eventHash []byte,
	state disposition,
) (event, bool) {
	input := candidate{
		tenantID: tenantID, auditEventID: eventID,
		principal: actor{typeID: actorType(principalType), id: actorID},
		action:    action(eventAction), outcome: outcome(eventOutcome), reason: reason(eventReason),
		target:        target{typeID: targetType(eventTargetType), id: targetID, version: cloneInt64(targetVersion)},
		policyVersion: policyVersion, requestID: requestID,
		approvalID: cloneString(approvalID), recoveryCaseID: cloneString(recoveryCaseID),
		evidenceDigest: bytes.Clone(evidenceDigest),
	}
	if contractVersion != 1 || sequence <= 0 || !recordedAt.Valid || !validCandidate(input) ||
		len(previousHash) != hashBytes || len(eventHash) != hashBytes {
		return event{}, false
	}
	return event{
		candidate: input, tenantSequence: sequence, recordedAt: recordedAt.Time.UTC(),
		previousEventHash: bytes.Clone(previousHash), eventHash: bytes.Clone(eventHash), disposition: state,
	}, true
}

func sameCandidate(left, right candidate) bool {
	return left.tenantID == right.tenantID && left.auditEventID == right.auditEventID &&
		left.principal == right.principal && left.action == right.action && left.outcome == right.outcome &&
		left.reason == right.reason && left.target.typeID == right.target.typeID &&
		left.target.id == right.target.id && equalInt64(left.target.version, right.target.version) &&
		left.policyVersion == right.policyVersion && left.requestID == right.requestID &&
		equalString(left.approvalID, right.approvalID) &&
		equalString(left.recoveryCaseID, right.recoveryCaseID) && bytes.Equal(left.evidenceDigest, right.evidenceDigest)
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func equalString(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func equalInt64(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func persistenceError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return storeError(errorPersistence)
}
