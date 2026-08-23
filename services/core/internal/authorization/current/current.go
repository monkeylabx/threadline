// Package current resolves and evaluates trustworthy current authorization
// facts inside a caller-owned PostgreSQL transaction.
package current

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/aclstore"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// ErrorCode classifies a stable current-authorization failure without exposing
// database or request data.
type ErrorCode string

const (
	// ErrorInvalidInput means the trusted resolver was called with an invalid
	// action, resource reference, or transaction.
	ErrorInvalidInput ErrorCode = "invalid-input"
	// ErrorPersistence means PostgreSQL could not supply trustworthy facts.
	ErrorPersistence ErrorCode = "persistence-failure"
)

// Error is a stable, secret-safe current-authorization error.
type Error struct{ code ErrorCode }

func (e *Error) Error() string { return "current authorization: " + string(e.Code()) }

// Code returns the stable failure category.
func (e *Error) Code() ErrorCode {
	if e == nil {
		return ""
	}
	return e.code
}

// EvaluateCurrent resolves one complete current fact snapshot and evaluates it.
// The caller retains ownership of tx and must finish the protected mutation
// before committing or rolling it back.
func EvaluateCurrent(
	ctx context.Context,
	tx pgx.Tx,
	action authorization.Action,
	ref authorization.ResourceRef,
) (authorization.Decision, error) {
	if err := ctx.Err(); err != nil {
		return authorization.Decision{}, err
	}
	initial := authorization.CurrentFacts{
		Resource: authorization.ResourceFacts{Ref: ref},
	}
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if !authenticated || principal.TenantID() != ref.TenantID {
		return authorization.Evaluate(ctx, action, initial)
	}
	if tx == nil {
		return authorization.Decision{}, &Error{code: ErrorInvalidInput}
	}
	if !knownAction(action) || !validResourceRef(ref) {
		return authorization.Decision{}, &Error{code: ErrorInvalidInput}
	}

	queries := dbgen.New(tx)
	isolation, err := queries.GetAuthorizationTransactionIsolation(ctx)
	if err != nil {
		return authorization.Decision{}, transactionError(ctx, err)
	}
	if isolation != "read committed" {
		return authorization.Decision{}, &Error{code: ErrorInvalidInput}
	}

	organization, err := queries.LockAuthorizationOrganization(ctx, ref.TenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.Evaluate(ctx, action, initial)
		}
		return authorization.Decision{}, persistenceError(ctx)
	}
	initial.Organization = authorization.OrganizationFacts{
		TenantID: organization.TenantID, State: organizationState(organization.State),
		PolicyVersion: organization.PolicyVersion,
	}
	if initial.Organization.State != authorization.OrganizationStateActive {
		initial.Available = true
		return authorization.Evaluate(ctx, action, initial)
	}

	member, err := queries.LockAuthorizationMember(ctx, dbgen.LockAuthorizationMemberParams{
		TenantID: ref.TenantID, ActorType: int16(principal.Actor().Type()), ActorID: principal.Actor().ID(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.Evaluate(ctx, action, initial)
		}
		return authorization.Decision{}, persistenceError(ctx)
	}
	initial.Member = authorization.MemberFacts{
		TenantID: member.TenantID,
		Actor: authorization.ActorRef{
			Type: rpcmiddleware.ActorType(member.ActorType), ID: member.ActorID,
		},
		State: memberState(member.State), Role: organizationRole(member.Role),
	}
	if initial.Member.State != authorization.MemberStateActive {
		initial.Available = true
		return authorization.Evaluate(ctx, action, initial)
	}

	switch ref.Kind {
	case authorization.ResourceKindSpace:
		space, lockErr := queries.LockAuthorizationSpace(ctx, dbgen.LockAuthorizationSpaceParams{
			TenantID: ref.TenantID, ResourceID: ref.ID,
		})
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return authorization.Evaluate(ctx, action, initial)
			}
			return authorization.Decision{}, persistenceError(ctx)
		}
		initial.Resource = authorization.ResourceFacts{
			Ref: authorization.ResourceRef{
				TenantID: space.TenantID, Kind: authorization.ResourceKindSpace, ID: space.SpaceID,
			},
			Exists: true,
		}
	case authorization.ResourceKindChannel:
		channel, lockErr := queries.LockAuthorizationChannel(ctx, dbgen.LockAuthorizationChannelParams{
			TenantID: ref.TenantID, ResourceID: ref.ID,
		})
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return authorization.Evaluate(ctx, action, initial)
			}
			return authorization.Decision{}, persistenceError(ctx)
		}
		initial.Resource = authorization.ResourceFacts{
			Ref: authorization.ResourceRef{
				TenantID: channel.TenantID, Kind: authorization.ResourceKindChannel, ID: channel.ChannelID,
			},
			Exists: true, Visibility: visibility(channel.Visibility), State: resourceState(channel.State),
		}
		membership, membershipErr := queries.LockActiveAuthorizationChannelMembership(
			ctx,
			dbgen.LockActiveAuthorizationChannelMembershipParams{
				TenantID: ref.TenantID, ChannelID: ref.ID,
				ActorType: int16(principal.Actor().Type()), ActorID: principal.Actor().ID(),
			},
		)
		initial.Membership = authorization.ChannelMembershipFacts{
			TenantID: ref.TenantID, ChannelID: ref.ID,
			Actor: authorization.ActorRef{Type: principal.Actor().Type(), ID: principal.Actor().ID()},
			Role:  authorization.ChannelRoleNone,
		}
		if membershipErr != nil {
			if !errors.Is(membershipErr, pgx.ErrNoRows) {
				return authorization.Decision{}, persistenceError(ctx)
			}
		} else {
			initial.Membership = authorization.ChannelMembershipFacts{
				TenantID: membership.TenantID, ChannelID: membership.ChannelID,
				Actor: authorization.ActorRef{
					Type: rpcmiddleware.ActorType(membership.ActorType), ID: membership.ActorID,
				},
				Current: true, IntervalID: strconv.FormatInt(membership.IntervalID, 10),
				Role: channelRole(membership.Role),
			}
		}
	}
	_, err = queries.LockCurrentAuthorizationACL(ctx, dbgen.LockCurrentAuthorizationACLParams{
		TenantID: ref.TenantID, ResourceKind: databaseResourceKind(ref.Kind), ResourceID: ref.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			initial.Available = true
			return authorization.Evaluate(ctx, action, initial)
		}
		return authorization.Decision{}, persistenceError(ctx)
	}

	acl, err := aclstore.LoadCurrent(ctx, tx, ref)
	if err != nil {
		var storeError *aclstore.Error
		if errors.As(err, &storeError) {
			switch storeError.Code() {
			case aclstore.ErrorCurrentNotFound:
				initial.Available = true
				return authorization.Evaluate(ctx, action, initial)
			case aclstore.ErrorInvalidStoredFacts:
				initial.ACL = acl
				initial.Available = true
				return authorization.Evaluate(ctx, action, initial)
			}
		}
		return authorization.Decision{}, persistenceError(ctx)
	}
	initial.ACL = acl
	initial.Available = true
	return authorization.Evaluate(ctx, action, initial)
}

func knownAction(action authorization.Action) bool {
	switch action {
	case authorization.ActionSpaceDiscover, authorization.ActionSpaceChannelCreate,
		authorization.ActionChannelDiscover, authorization.ActionChannelRead,
		authorization.ActionChannelPublish, authorization.ActionChannelUpdate,
		authorization.ActionChannelArchive, authorization.ActionChannelMembershipList,
		authorization.ActionChannelMembershipAdd, authorization.ActionChannelMembershipRemove,
		authorization.ActionChannelACLUpdate:
		return true
	default:
		return false
	}
}

func validResourceRef(ref authorization.ResourceRef) bool {
	if !validID(ref.TenantID) || !validID(ref.ID) {
		return false
	}
	return ref.Kind == authorization.ResourceKindSpace || ref.Kind == authorization.ResourceKindChannel
}

func databaseResourceKind(kind authorization.ResourceKind) int16 {
	if kind == authorization.ResourceKindSpace {
		return 1
	}
	return 2
}

func validID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func organizationState(state int16) authorization.OrganizationState {
	if state == 1 {
		return authorization.OrganizationStateActive
	}
	return ""
}

func memberState(state int16) authorization.MemberState {
	if state == 2 {
		return authorization.MemberStateActive
	}
	return ""
}

func organizationRole(role int16) authorization.OrganizationRole {
	switch role {
	case 1:
		return authorization.OrganizationRoleOwner
	case 2:
		return authorization.OrganizationRoleAdmin
	case 3:
		return authorization.OrganizationRoleSecurityAdmin
	case 4:
		return authorization.OrganizationRoleMember
	case 5:
		return authorization.OrganizationRoleGuest
	default:
		return ""
	}
}

func visibility(value int16) authorization.Visibility {
	switch value {
	case 1:
		return authorization.VisibilityPublic
	case 2:
		return authorization.VisibilityPrivate
	default:
		return ""
	}
}

func resourceState(state int16) authorization.ResourceState {
	switch state {
	case 1:
		return authorization.ResourceStateActive
	case 2:
		return authorization.ResourceStateArchived
	case 3:
		return authorization.ResourceStatePendingDeletion
	default:
		return ""
	}
}

func channelRole(role int16) authorization.ChannelRole {
	switch role {
	case 1:
		return authorization.ChannelRoleOwner
	case 2:
		return authorization.ChannelRoleModerator
	case 3:
		return authorization.ChannelRoleMember
	case 4:
		return authorization.ChannelRoleGuest
	default:
		return ""
	}
}

func transactionError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, pgx.ErrTxClosed) {
		return &Error{code: ErrorInvalidInput}
	}
	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) && sqlState.SQLState() == "25P02" {
		return &Error{code: ErrorInvalidInput}
	}
	return &Error{code: ErrorPersistence}
}

func persistenceError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return &Error{code: ErrorPersistence}
}
