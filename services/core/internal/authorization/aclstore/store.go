// Package aclstore persists immutable, versioned Resource ACL snapshots.
package aclstore

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// ErrorCode classifies a stable ACL-store failure without exposing SQL data.
type ErrorCode string

const (
	ErrorInvalidInput       ErrorCode = "invalid-input"
	ErrorCurrentNotFound    ErrorCode = "current-not-found"
	ErrorResourceNotFound   ErrorCode = "resource-not-found"
	ErrorInvalidStoredFacts ErrorCode = "invalid-stored-facts"
	ErrorPersistence        ErrorCode = "persistence-failure"
)

// Error is a stable, secret-safe ACL-store error.
type Error struct{ code ErrorCode }

func (e *Error) Error() string { return "resource ACL store: " + string(e.Code()) }

// Code returns the stable failure category.
func (e *Error) Code() ErrorCode {
	if e == nil {
		return ""
	}
	return e.code
}

// Replacement is a complete ACL snapshot. Version is intentionally absent;
// PostgreSQL generates it when ReplaceCurrent appends the snapshot.
type Replacement struct {
	Resource      authorization.ResourceRef
	DefaultEffect authorization.ACLEffect
	Entries       []authorization.ACLEntry
}

// LoadCurrent loads one complete immutable current ACL snapshot. It never
// synthesizes a default ACL when no current head exists.
func LoadCurrent(
	ctx context.Context,
	tx pgx.Tx,
	ref authorization.ResourceRef,
) (authorization.ResourceACLFacts, error) {
	if err := ctx.Err(); err != nil {
		return authorization.ResourceACLFacts{}, err
	}
	resourceKind, _, _, ok := databaseResource(ref)
	if tx == nil || !ok {
		return authorization.ResourceACLFacts{}, storeError(ErrorInvalidInput)
	}
	queries := dbgen.New(tx)
	snapshot, err := queries.GetCurrentResourceACLSnapshot(ctx, dbgen.GetCurrentResourceACLSnapshotParams{
		TenantID: ref.TenantID, ResourceKind: resourceKind, ResourceID: ref.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.ResourceACLFacts{}, storeError(ErrorCurrentNotFound)
		}
		return authorization.ResourceACLFacts{}, persistenceError(ctx)
	}
	rows, err := queries.ListResourceACLEntries(ctx, dbgen.ListResourceACLEntriesParams{
		TenantID: ref.TenantID, AclVersion: snapshot.AclVersion,
	})
	if err != nil {
		return authorization.ResourceACLFacts{}, persistenceError(ctx)
	}
	facts := authorization.ResourceACLFacts{
		Resource: ref, Version: strconv.FormatInt(snapshot.AclVersion, 10),
	}
	invalidStoredFacts := false
	entries := make([]authorization.ACLEntry, len(rows))
	for index, row := range rows {
		entry, ok := authorizationEntry(row.ActorType, row.ActorID, row.Action, row.Effect)
		if !ok {
			invalidStoredFacts = true
		}
		entries[index] = entry
	}
	facts.Entries = entries
	defaultEffect, ok := authorizationEffect(snapshot.DefaultEffect)
	if !ok {
		invalidStoredFacts = true
	}
	facts.DefaultEffect = defaultEffect
	if invalidStoredFacts {
		return facts, storeError(ErrorInvalidStoredFacts)
	}
	return facts, nil
}

// ReplaceCurrent appends, seals, and makes current one complete snapshot inside
// the caller-owned transaction. The caller must commit success or roll back any
// error; this function never controls transaction lifetime.
func ReplaceCurrent(
	ctx context.Context,
	tx pgx.Tx,
	replacement Replacement,
) (authorization.ResourceACLFacts, error) {
	if err := ctx.Err(); err != nil {
		return authorization.ResourceACLFacts{}, err
	}
	resourceKind, spaceID, channelID, ok := databaseResource(replacement.Resource)
	defaultEffect, effectOK := databaseEffect(replacement.DefaultEffect)
	entries, entriesOK := validatedEntries(replacement.Entries)
	if tx == nil || !ok || !effectOK || !entriesOK {
		return authorization.ResourceACLFacts{}, storeError(ErrorInvalidInput)
	}
	queries := dbgen.New(tx)
	var lockErr error
	if replacement.Resource.Kind == authorization.ResourceKindSpace {
		_, lockErr = queries.LockSpaceForACLReplacement(ctx, dbgen.LockSpaceForACLReplacementParams{
			TenantID: replacement.Resource.TenantID, ResourceID: replacement.Resource.ID,
		})
	} else {
		_, lockErr = queries.LockChannelForACLReplacement(ctx, dbgen.LockChannelForACLReplacementParams{
			TenantID: replacement.Resource.TenantID, ResourceID: replacement.Resource.ID,
		})
	}
	if lockErr != nil {
		if errors.Is(lockErr, pgx.ErrNoRows) {
			return authorization.ResourceACLFacts{}, storeError(ErrorResourceNotFound)
		}
		return authorization.ResourceACLFacts{}, persistenceError(ctx)
	}
	snapshot, err := queries.CreateResourceACLSnapshot(ctx, dbgen.CreateResourceACLSnapshotParams{
		TenantID: replacement.Resource.TenantID, ResourceKind: resourceKind,
		ResourceID: replacement.Resource.ID, SpaceID: spaceID, ChannelID: channelID,
		DefaultEffect: defaultEffect,
	})
	if err != nil {
		return authorization.ResourceACLFacts{}, persistenceError(ctx)
	}
	for index, entry := range entries {
		action, _ := databaseAction(entry.Action)
		effect, _ := databaseEffect(entry.Effect)
		if err := queries.CreateResourceACLEntry(ctx, dbgen.CreateResourceACLEntryParams{
			TenantID: replacement.Resource.TenantID, AclVersion: snapshot.AclVersion,
			EntryOrdinal: int32(index + 1), ActorType: int16(entry.Actor.Type),
			ActorID: entry.Actor.ID, Action: action, Effect: effect,
		}); err != nil {
			return authorization.ResourceACLFacts{}, persistenceError(ctx)
		}
	}
	if _, err := queries.SealResourceACLSnapshot(ctx, dbgen.SealResourceACLSnapshotParams{
		TenantID: replacement.Resource.TenantID, AclVersion: snapshot.AclVersion,
	}); err != nil {
		return authorization.ResourceACLFacts{}, persistenceError(ctx)
	}
	if err := queries.SetCurrentResourceACL(ctx, dbgen.SetCurrentResourceACLParams{
		TenantID: replacement.Resource.TenantID, ResourceKind: resourceKind,
		ResourceID: replacement.Resource.ID, SpaceID: spaceID, ChannelID: channelID,
		AclVersion: snapshot.AclVersion,
	}); err != nil {
		return authorization.ResourceACLFacts{}, persistenceError(ctx)
	}
	return LoadCurrent(ctx, tx, replacement.Resource)
}

func databaseResource(ref authorization.ResourceRef) (int16, *string, *string, bool) {
	if !validID(ref.TenantID) || !validID(ref.ID) {
		return 0, nil, nil, false
	}
	switch ref.Kind {
	case authorization.ResourceKindSpace:
		return 1, &ref.ID, nil, true
	case authorization.ResourceKindChannel:
		return 2, nil, &ref.ID, true
	default:
		return 0, nil, nil, false
	}
}

func databaseEffect(effect authorization.ACLEffect) (int16, bool) {
	switch effect {
	case authorization.ACLEffectAllow:
		return 1, true
	case authorization.ACLEffectDeny:
		return 2, true
	default:
		return 0, false
	}
}

func authorizationEffect(effect int16) (authorization.ACLEffect, bool) {
	switch effect {
	case 1:
		return authorization.ACLEffectAllow, true
	case 2:
		return authorization.ACLEffectDeny, true
	default:
		return "", false
	}
}

func databaseAction(action authorization.Action) (int16, bool) {
	switch action {
	case authorization.ActionSpaceDiscover:
		return 1, true
	case authorization.ActionSpaceChannelCreate:
		return 2, true
	case authorization.ActionChannelDiscover:
		return 3, true
	case authorization.ActionChannelRead:
		return 4, true
	case authorization.ActionChannelPublish:
		return 5, true
	case authorization.ActionChannelUpdate:
		return 6, true
	case authorization.ActionChannelArchive:
		return 7, true
	case authorization.ActionChannelMembershipList:
		return 8, true
	case authorization.ActionChannelMembershipAdd:
		return 9, true
	case authorization.ActionChannelMembershipRemove:
		return 10, true
	case authorization.ActionChannelACLUpdate:
		return 11, true
	default:
		return 0, false
	}
}

func authorizationAction(action int16) (authorization.Action, bool) {
	for _, candidate := range [...]authorization.Action{
		authorization.ActionSpaceDiscover, authorization.ActionSpaceChannelCreate,
		authorization.ActionChannelDiscover, authorization.ActionChannelRead,
		authorization.ActionChannelPublish, authorization.ActionChannelUpdate,
		authorization.ActionChannelArchive, authorization.ActionChannelMembershipList,
		authorization.ActionChannelMembershipAdd, authorization.ActionChannelMembershipRemove,
		authorization.ActionChannelACLUpdate,
	} {
		encoded, _ := databaseAction(candidate)
		if encoded == action {
			return candidate, true
		}
	}
	return "", false
}

func validatedEntries(input []authorization.ACLEntry) ([]authorization.ACLEntry, bool) {
	entries := append([]authorization.ACLEntry(nil), input...)
	for _, entry := range entries {
		if !knownActorType(entry.Actor.Type) || !validID(entry.Actor.ID) {
			return nil, false
		}
		if _, ok := databaseAction(entry.Action); !ok {
			return nil, false
		}
		if _, ok := databaseEffect(entry.Effect); !ok {
			return nil, false
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Actor.Type != entries[right].Actor.Type {
			return entries[left].Actor.Type < entries[right].Actor.Type
		}
		if entries[left].Actor.ID != entries[right].Actor.ID {
			return entries[left].Actor.ID < entries[right].Actor.ID
		}
		if entries[left].Action != entries[right].Action {
			leftAction, _ := databaseAction(entries[left].Action)
			rightAction, _ := databaseAction(entries[right].Action)
			return leftAction < rightAction
		}
		return entries[left].Effect < entries[right].Effect
	})
	for index := 1; index < len(entries); index++ {
		if entries[index] == entries[index-1] {
			return nil, false
		}
	}
	return entries, true
}

func authorizationEntry(actorType int16, actorID string, action, effect int16) (authorization.ACLEntry, bool) {
	entry := authorization.ACLEntry{
		Actor: authorization.ActorRef{Type: rpcmiddleware.ActorType(actorType), ID: actorID},
	}
	actorIDValid := validID(actorID)
	if !actorIDValid {
		entry.Actor.ID = ""
	}
	valid := knownActorType(entry.Actor.Type) && actorIDValid
	decodedAction, actionOK := authorizationAction(action)
	decodedEffect, effectOK := authorizationEffect(effect)
	if actionOK {
		entry.Action = decodedAction
	}
	if effectOK {
		entry.Effect = decodedEffect
	}
	return entry, valid && actionOK && effectOK
}

func knownActorType(actorType rpcmiddleware.ActorType) bool {
	return actorType == rpcmiddleware.ActorTypeHuman || actorType == rpcmiddleware.ActorTypeAgent || actorType == rpcmiddleware.ActorTypeService
}

func validID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func persistenceError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return storeError(ErrorPersistence)
}

func storeError(code ErrorCode) error { return &Error{code: code} }
