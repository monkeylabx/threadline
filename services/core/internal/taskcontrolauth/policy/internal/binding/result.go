// Package binding validates that typed authorization inputs bind to the
// authenticated principal and addressed resource in contract refusal order.
package binding

import "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"

const (
	ReasonAuthenticationRequired model.Reason = "authentication-required"
	ReasonShapeInvalid           model.Reason = "shape-invalid"
	ReasonUnknownAction          model.Reason = "unknown-action"
	ReasonPurposeInvalid         model.Reason = "purpose-invalid"
	ReasonUnsupportedParent      model.Reason = "unsupported-parent"
	ReasonFactsUnavailable       model.Reason = "facts-unavailable"
	ReasonFactsBindingInvalid    model.Reason = "facts-binding-invalid"
	ReasonDeviceRevoked          model.Reason = "device-revoked"
	ReasonOrganizationInactive   model.Reason = "organization-inactive"
	ReasonMemberInactive         model.Reason = "member-inactive"
	ReasonObjectAbsent           model.Reason = "object-absent"
	ReasonPolicyUnavailable      model.Reason = "policy-unavailable"
	ReasonPolicyInvalid          model.Reason = "policy-invalid"
	ReasonNotAMember             model.Reason = "not-a-member"
	ReasonParentReadDenied       model.Reason = "parent-read-denied"
	ReasonParentACLDenied        model.Reason = "parent-acl-denied"
	ReasonReceiptNamespaceDenied model.Reason = "receipt-namespace-denied"
	ReasonIdempotencyConflict    model.Reason = "idempotency-conflict"
)

// Result holds either a refusal or the validated resource scope and metadata.
// A zero Refusal means inputs are bound; it never grants an operation.
type Result struct {
	Refusal         model.Refusal
	Scope           model.Scope
	TaskRole        model.TaskRole
	EffectiveAction model.Action
	ReceiptSelected bool
}

func refuse(reason model.Reason, status model.Status, code model.Code) model.Refusal {
	return model.Refusal{Reason: reason, Status: status, Code: code}
}
