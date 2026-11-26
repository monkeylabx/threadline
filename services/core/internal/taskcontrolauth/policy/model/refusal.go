package model

type Status string
type Code string
type Reason string

const (
	StatusUnauthenticated      Status = "UNAUTHENTICATED"
	StatusNotFound             Status = "NOT_FOUND"
	StatusUnimplemented        Status = "UNIMPLEMENTED"
	StatusInvalidArgument      Status = "INVALID_ARGUMENT"
	StatusUnavailable          Status = "UNAVAILABLE"
	StatusPermissionDenied     Status = "PERMISSION_DENIED"
	StatusFailedPrecondition   Status = "FAILED_PRECONDITION"
	StatusAlreadyExists        Status = "ALREADY_EXISTS"
	StatusResourceExhausted    Status = "RESOURCE_EXHAUSTED"
	CodePermissionDenied       Code   = "PERMISSION_DENIED"
	CodeNotAMember             Code   = "NOT_A_MEMBER"
	CodeHistorySharingDenied   Code   = "HISTORY_SHARING_DENIED"
	CodeDeviceRevoked          Code   = "DEVICE_REVOKED"
	CodeRetentionExpired       Code   = "RETENTION_EXPIRED"
	CodeGrantExpired           Code   = "GRANT_EXPIRED"
	CodeGrantRevoked           Code   = "GRANT_REVOKED"
	CodeGrantOutOfScope        Code   = "GRANT_OUT_OF_SCOPE"
	CodeApprovalRequired       Code   = "APPROVAL_REQUIRED"
	CodeNotExecutionOwner      Code   = "NOT_EXECUTION_OWNER"
	CodeLeaseLost              Code   = "LEASE_LOST"
	CodeFencingTokenStale      Code   = "FENCING_TOKEN_STALE"
	CodeInvalidStateTransition Code   = "INVALID_STATE_TRANSITION"
	CodeIdempotencyConflict    Code   = "IDEMPOTENCY_CONFLICT"
	CodePayloadTooLarge        Code   = "PAYLOAD_TOO_LARGE"
)

type Refusal struct {
	Reason Reason
	Status Status
	Code   Code
}

func (r Refusal) Refused() bool { return r.Status != "" }
