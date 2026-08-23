package outboxdb

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monkeylabx/threadline/services/worker/internal/dbgen"
)

const (
	payloadHardBytes = 262_144
	policyV1ID       = "threadline.outbox.policy/v1"
)

type outboxQueries interface {
	ClaimTransactionalOutboxBatch(context.Context, dbgen.ClaimTransactionalOutboxBatchParams) ([]dbgen.ClaimTransactionalOutboxBatchRow, error)
	RenewTransactionalOutboxClaim(context.Context, dbgen.RenewTransactionalOutboxClaimParams) (dbgen.RenewTransactionalOutboxClaimRow, error)
	AcknowledgeTransactionalOutboxPublished(context.Context, dbgen.AcknowledgeTransactionalOutboxPublishedParams) (string, error)
	RecordTransactionalOutboxPublishFailure(context.Context, dbgen.RecordTransactionalOutboxPublishFailureParams) (dbgen.RecordTransactionalOutboxPublishFailureRow, error)
}

// adapter binds the reviewed operations to a caller-owned pgx transaction or
// connection. It has no transaction-lifetime methods and never commits,
// rolls back, or opens a transaction for its caller.
type adapter struct{ queries outboxQueries }

func newAdapter(database dbgen.DBTX) (*adapter, error) {
	if database == nil {
		return nil, operationError(errorInvalidInput)
	}
	return &adapter{queries: dbgen.New(database)}, nil
}

func (bound *adapter) claim(ctx context.Context, request claimRequest) ([]claimedEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if bound == nil || bound.queries == nil || !validClaimRequest(request) {
		return nil, operationError(errorInvalidInput)
	}

	rows, err := bound.queries.ClaimTransactionalOutboxBatch(ctx, dbgen.ClaimTransactionalOutboxBatchParams{
		ClaimOwnerID: request.claimOwnerID,
		BatchSize:    request.batchSize,
	})
	if err != nil {
		return nil, databaseFailure(ctx, err)
	}
	defer func() {
		for index := range rows {
			clear(rows[index].RawClaimToken)
		}
	}()
	if len(rows) > int(request.batchSize) {
		return nil, operationError(errorPersistence)
	}

	claimed := make([]claimedEvent, 0, len(rows))
	for _, row := range rows {
		mapped, ok := claimedEventFromRow(row, request.claimOwnerID)
		if !ok {
			return nil, operationError(errorPersistence)
		}
		claimed = append(claimed, mapped)
	}
	return claimed, nil
}

func (bound *adapter) renew(ctx context.Context, fence claimFence) (renewal, error) {
	if err := ctx.Err(); err != nil {
		return renewal{}, err
	}
	if bound == nil || bound.queries == nil {
		return renewal{}, operationError(errorInvalidInput)
	}
	if !validFenceFacts(fence) {
		return renewal{}, operationError(errorClaimDenied)
	}
	digest, ok := fence.token.candidateDigest()
	if !ok {
		return renewal{}, operationError(errorClaimDenied)
	}
	candidateDigest := bytes.Clone(digest[:])
	clear(digest[:])
	defer clear(candidateDigest)

	row, err := bound.queries.RenewTransactionalOutboxClaim(ctx, dbgen.RenewTransactionalOutboxClaimParams{
		TenantID:          fence.tenantID,
		EventID:           fence.eventID,
		OutboxEntryID:     fence.outboxEntryID,
		DeliveryAttemptID: fence.deliveryAttemptID,
		ReplayGeneration:  fence.replayGeneration,
		ClaimOwnerID:      fence.claimOwnerID,
		CandidateDigest:   candidateDigest,
	})
	if err != nil {
		return renewal{}, databaseFailure(ctx, err)
	}
	switch row.ResultCode {
	case "renewed":
		if !row.LeaseExpiresAt.Valid {
			return renewal{}, operationError(errorPersistence)
		}
		return renewal{leaseExpiresAt: row.LeaseExpiresAt.Time.UTC()}, nil
	case "claim-denied":
		if row.LeaseExpiresAt.Valid {
			return renewal{}, operationError(errorPersistence)
		}
		return renewal{}, operationError(errorClaimDenied)
	default:
		return renewal{}, operationError(errorPersistence)
	}
}

func (bound *adapter) acknowledge(ctx context.Context, request acknowledgementRequest) (acknowledgement, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if bound == nil || bound.queries == nil {
		return 0, operationError(errorInvalidInput)
	}
	if !validFenceFacts(request.fence) {
		return 0, operationError(errorClaimDenied)
	}
	if !validPubAck(request.pubAck) {
		return 0, operationError(errorInvalidInput)
	}
	digest, ok := request.fence.token.candidateDigest()
	if !ok {
		return 0, operationError(errorClaimDenied)
	}
	candidateDigest := bytes.Clone(digest[:])
	clear(digest[:])
	defer clear(candidateDigest)

	resultCode, err := bound.queries.AcknowledgeTransactionalOutboxPublished(ctx, dbgen.AcknowledgeTransactionalOutboxPublishedParams{
		TenantID:          request.fence.tenantID,
		EventID:           request.fence.eventID,
		OutboxEntryID:     request.fence.outboxEntryID,
		DeliveryAttemptID: request.fence.deliveryAttemptID,
		ReplayGeneration:  request.fence.replayGeneration,
		ClaimOwnerID:      request.fence.claimOwnerID,
		CandidateDigest:   candidateDigest,
		BrokerStream:      request.pubAck.stream,
		BrokerSequence: pgtype.Numeric{
			Int:   new(big.Int).SetUint64(request.pubAck.sequence),
			Valid: true,
		},
		BrokerDuplicate: request.pubAck.duplicate,
		BrokerMessageID: request.pubAck.messageID,
	})
	if err != nil {
		return 0, databaseFailure(ctx, err)
	}
	switch resultCode {
	case "delivered":
		return acknowledgementDelivered, nil
	case "already-delivered":
		return acknowledgementAlreadyDelivered, nil
	case "claim-denied":
		return 0, operationError(errorClaimDenied)
	default:
		return 0, operationError(errorPersistence)
	}
}

func (bound *adapter) recordFailure(ctx context.Context, request publishFailureRequest) (failureResult, error) {
	if err := ctx.Err(); err != nil {
		return failureResult{}, err
	}
	if bound == nil || bound.queries == nil {
		return failureResult{}, operationError(errorInvalidInput)
	}
	if !validFenceFacts(request.fence) {
		return failureResult{}, operationError(errorClaimDenied)
	}
	if !validFailureCode(request.code) {
		return failureResult{}, operationError(errorInvalidInput)
	}
	digest, ok := request.fence.token.candidateDigest()
	if !ok {
		return failureResult{}, operationError(errorClaimDenied)
	}
	candidateDigest := bytes.Clone(digest[:])
	clear(digest[:])
	defer clear(candidateDigest)

	row, err := bound.queries.RecordTransactionalOutboxPublishFailure(ctx, dbgen.RecordTransactionalOutboxPublishFailureParams{
		TenantID:          request.fence.tenantID,
		EventID:           request.fence.eventID,
		OutboxEntryID:     request.fence.outboxEntryID,
		DeliveryAttemptID: request.fence.deliveryAttemptID,
		ReplayGeneration:  request.fence.replayGeneration,
		ClaimOwnerID:      request.fence.claimOwnerID,
		CandidateDigest:   candidateDigest,
		FailureCode:       string(request.code),
	})
	if err != nil {
		return failureResult{}, databaseFailure(ctx, err)
	}
	switch row.ResultCode {
	case "retry-scheduled":
		if !row.NextAttemptAt.Valid {
			return failureResult{}, operationError(errorPersistence)
		}
		return failureResult{
			disposition:   failureDispositionRetryScheduled,
			nextAttemptAt: row.NextAttemptAt.Time.UTC(),
		}, nil
	case "parked":
		if row.NextAttemptAt.Valid {
			return failureResult{}, operationError(errorPersistence)
		}
		return failureResult{disposition: failureDispositionParked}, nil
	case "claim-denied":
		if row.NextAttemptAt.Valid {
			return failureResult{}, operationError(errorPersistence)
		}
		return failureResult{}, operationError(errorClaimDenied)
	default:
		return failureResult{}, operationError(errorPersistence)
	}
}

func claimedEventFromRow(row dbgen.ClaimTransactionalOutboxBatchRow, expectedOwner string) (claimedEvent, bool) {
	token, validToken := newClaimToken(row.RawClaimToken)
	if row.ResultCode != "claimed" || row.ClaimOwnerID != expectedOwner || !validToken ||
		!validBoundedIdentifier(row.TenantID, 0) || !validBoundedIdentifier(row.EventID, 0) ||
		row.OutboxEntryID <= 0 || row.DeliveryAttemptID <= 0 || row.ReplayGeneration < 0 ||
		row.TotalAttemptNumber <= 0 || row.GenerationAttemptNumber <= 0 || row.GenerationAttemptNumber > row.TotalAttemptNumber ||
		!row.ClaimedAt.Valid || !row.LeaseExpiresAt.Valid || !row.AbsoluteLeaseExpiresAt.Valid ||
		!row.ClaimedAt.Time.Before(row.LeaseExpiresAt.Time) || row.LeaseExpiresAt.Time.After(row.AbsoluteLeaseExpiresAt.Time) ||
		!validMessageID(row.BrokerMessageID) || row.Destination != "domain-events" ||
		!validEventType(row.EventType) || row.SchemaVersion <= 0 ||
		!validBoundedIdentifier(row.AggregateKind, 0) || !validBoundedIdentifier(row.AggregateID, 0) ||
		len(row.Payload) > payloadHardBytes || !row.OccurredAt.Valid || !row.EnqueuedAt.Valid ||
		row.PolicyID != policyV1ID || len(row.PolicySnapshotDigest) != policyDigestBytes {
		return claimedEvent{}, false
	}

	var policyDigest [policyDigestBytes]byte
	copy(policyDigest[:], row.PolicySnapshotDigest)
	return claimedEvent{
		fence: claimFence{
			tenantID:          row.TenantID,
			eventID:           row.EventID,
			outboxEntryID:     row.OutboxEntryID,
			deliveryAttemptID: row.DeliveryAttemptID,
			replayGeneration:  row.ReplayGeneration,
			claimOwnerID:      row.ClaimOwnerID,
			token:             token,
		},
		totalAttemptNumber:      row.TotalAttemptNumber,
		generationAttemptNumber: row.GenerationAttemptNumber,
		claimedAt:               row.ClaimedAt.Time.UTC(),
		leaseExpiresAt:          row.LeaseExpiresAt.Time.UTC(),
		absoluteLeaseExpiresAt:  row.AbsoluteLeaseExpiresAt.Time.UTC(),
		brokerMessageID:         row.BrokerMessageID,
		destination:             row.Destination,
		eventType:               row.EventType,
		schemaVersion:           row.SchemaVersion,
		aggregateKind:           row.AggregateKind,
		aggregateID:             row.AggregateID,
		payload:                 bytes.Clone(row.Payload),
		occurredAt:              row.OccurredAt.Time.UTC(),
		enqueuedAt:              row.EnqueuedAt.Time.UTC(),
		policyID:                row.PolicyID,
		policySnapshotDigest:    policyDigest,
	}, true
}

func validEventType(value string) bool {
	dot := strings.IndexByte(value, '.')
	return validBoundedIdentifier(value, 0) && value == strings.ToLower(value) && dot > 0 && dot < len(value)-1
}

func databaseFailure(ctx context.Context, err error) error {
	if contextError := ctx.Err(); contextError != nil {
		return contextError
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}

	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Message {
		case "transactional outbox: invalid-input":
			return operationError(errorInvalidInput)
		case "transactional outbox: persistence-failure":
			return operationError(errorPersistence)
		}
	}
	return operationError(errorPersistence)
}
