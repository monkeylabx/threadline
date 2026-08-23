package dbgen

// Token-bearing sqlc values are deliberately non-renderable. They exist only
// long enough for the private Worker adapter to copy or hash their authority
// bytes and clear the database-owned buffers.
const redactedOutboxAuthority = "[redacted-outbox-authority]"

func (ClaimTransactionalOutboxBatchRow) String() string   { return redactedOutboxAuthority }
func (ClaimTransactionalOutboxBatchRow) GoString() string { return redactedOutboxAuthority }
func (ClaimTransactionalOutboxBatchRow) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-authority]"`), nil
}

func (RenewTransactionalOutboxClaimParams) String() string   { return redactedOutboxAuthority }
func (RenewTransactionalOutboxClaimParams) GoString() string { return redactedOutboxAuthority }
func (RenewTransactionalOutboxClaimParams) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-authority]"`), nil
}

func (AcknowledgeTransactionalOutboxPublishedParams) String() string {
	return redactedOutboxAuthority
}
func (AcknowledgeTransactionalOutboxPublishedParams) GoString() string {
	return redactedOutboxAuthority
}
func (AcknowledgeTransactionalOutboxPublishedParams) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-authority]"`), nil
}

func (RecordTransactionalOutboxPublishFailureParams) String() string {
	return redactedOutboxAuthority
}
func (RecordTransactionalOutboxPublishFailureParams) GoString() string {
	return redactedOutboxAuthority
}
func (RecordTransactionalOutboxPublishFailureParams) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-authority]"`), nil
}
