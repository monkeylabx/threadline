package model

type Purpose string
type ParentKind string
type ResourceKind string

const (
	PurposeOperation      Purpose      = "operation"
	PurposeOwnReceiptRead Purpose      = "own_receipt_read"
	ParentChannel         ParentKind   = "channel"
	ResourceChannel       ResourceKind = "channel"
	ResourceTask          ResourceKind = "task"
	ResourceRun           ResourceKind = "run"
	ResourceApproval      ResourceKind = "approval"
)

type ResourceRef struct {
	Kind ResourceKind
	ID   string
}

type Ancestry struct {
	Channel, Task, Run, Approval string
}

type Request struct {
	Action        Action
	Purpose       Purpose
	ParentKind    ParentKind
	Resource      ResourceRef
	Ancestry      Ancestry // caller equality assertions only
	RPC           string   // receipt namespace
	RequestDigest [32]byte
}
