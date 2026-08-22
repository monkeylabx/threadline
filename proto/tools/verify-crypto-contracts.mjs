import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixtureRoot = join(repositoryRoot, "test", "fixtures", "proto", "crypto");
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "manifest.json"), "utf8"));
const scenarioBytes = readFileSync(join(fixtureRoot, manifest.source.file));
const scenarios = JSON.parse(scenarioBytes.toString("utf8"));

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function read(relativePath) {
  return readFileSync(join(repositoryRoot, relativePath), "utf8").replaceAll("\r\n", "\n");
}

function merge(base, overlay) {
  if (Array.isArray(overlay) || overlay === null || typeof overlay !== "object") return overlay;
  const result = { ...(base ?? {}) };
  for (const [key, value] of Object.entries(overlay)) result[key] = merge(base?.[key], value);
  return result;
}

function reject(errorCode, extra = {}) {
  return { status: "rejected", errorCode, ...extra };
}

function evaluateCredential(testCase) {
  const { credential, now } = testCase;
  const allowedIssuers = new Set([
    "CREDENTIAL_ISSUER_KIND_DEVICE_AUTHORITY",
    "CREDENTIAL_ISSUER_KIND_EXISTING_AUTHORIZED_DEVICE",
    "CREDENTIAL_ISSUER_KIND_ADMIN_EXCEPTION",
  ]);
  if (credential.tenantId !== "tenant-a") return reject("ERROR_CODE_TENANT_MISMATCH");
  if (credential.deviceId !== "device-alice-1" || credential.actorId !== "actor-alice") return reject("ERROR_CODE_DEVICE_NOT_ENROLLED");
  if (credential.credentialFormatVersion !== 1) return reject("ERROR_CODE_ENVELOPE_VERSION_UNSUPPORTED");
  if (credential.profile !== "tl-mls-1") return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
  if (!allowedIssuers.has(credential.issuerKind)
    || !credential.issuerAuthorized
    || !credential.approvalSignatureValid
    || credential.issuerKind === "CREDENTIAL_ISSUER_KIND_UNSPECIFIED"
    || credential.issuerKind !== credential.signedIssuerKind
    || credential.approvedBy !== credential.signedApprovedBy) {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (now > credential.expiresAt) return reject("ERROR_CODE_DEVICE_REVOKED");
  return { status: "accepted" };
}

function evaluateKeyPackage(testCase) {
  const { keyPackage, claim, legacyKeyPackageContext, now } = testCase;
  const additivePublicationFields = [
    keyPackage.tenantId,
    keyPackage.credentialVersion,
    keyPackage.notBefore,
    keyPackage.issuedAt,
    keyPackage.state,
    keyPackage.publicationBindingHash,
    keyPackage.publicationSignature,
  ];
  const isLegacyPackage = additivePublicationFields.every((value) => value === null);
  if (isLegacyPackage) {
    if (!legacyKeyPackageContext.publishSessionAuthorized) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (legacyKeyPackageContext.publishSessionTenantId !== claim.tenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
    if (keyPackage.deviceId !== legacyKeyPackageContext.publishSessionDeviceId
      || keyPackage.deviceId !== claim.deviceId) return reject("ERROR_CODE_DEVICE_NOT_ENROLLED");
    if (!legacyKeyPackageContext.publishSessionProfiles.includes(keyPackage.profile)
      || keyPackage.profile !== claim.profile) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
    if (!keyPackage.available
      || now < keyPackage.publishedAt
      || now > keyPackage.expiresAt
      || keyPackage.expiresAt - keyPackage.publishedAt > 7261200) return reject("ERROR_CODE_KEY_PACKAGE_UNAVAILABLE");
    if (legacyKeyPackageContext.clientCryptoValidation !== "accepted") {
      return reject("ERROR_CODE_CIPHERTEXT_CORRUPT", {
        atomicClaim: true,
        packageTerminalState: "KEY_PACKAGE_STATE_CONSUMED",
        commitProduced: false,
      });
    }
    return { status: "claimed", claimId: claim.claimId, legacyMigration: true, atomicClaim: true };
  }
  if (additivePublicationFields.some((value) => value === null)) {
    return { status: "rejected", errorCode: "ERROR_CODE_KEY_PACKAGE_UNAVAILABLE", action: "republish" };
  }
  const publicationProjection = {
    keyPackageId: keyPackage.keyPackageId,
    deviceId: keyPackage.deviceId,
    keyPackageData: keyPackage.keyPackageData,
    cryptoProfile: keyPackage.profile,
    publishedAt: keyPackage.publishedAt,
    expiresAt: keyPackage.expiresAt,
    tenantId: keyPackage.tenantId,
    credentialVersion: keyPackage.credentialVersion,
    notBefore: keyPackage.notBefore,
    issuedAt: keyPackage.issuedAt,
  };
  const expectedPublicationBinding = sha256(Buffer.from(JSON.stringify(publicationProjection)));
  const expectedPublicationSignature = sha256(Buffer.from(`${keyPackage.deviceId}:${expectedPublicationBinding}`));
  if (keyPackage.publicationBindingHash !== expectedPublicationBinding
    || keyPackage.publicationSignature !== expectedPublicationSignature) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (keyPackage.tenantId !== claim.tenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
  if (keyPackage.deviceId !== claim.deviceId) return reject("ERROR_CODE_DEVICE_NOT_ENROLLED");
  if (keyPackage.credentialVersion !== claim.credentialVersion) return reject("ERROR_CODE_DEVICE_REVOKED");
  if (keyPackage.profile !== claim.profile || keyPackage.profile !== "tl-mls-1") return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
  if (keyPackage.available !== (keyPackage.state === "KEY_PACKAGE_STATE_AVAILABLE")) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (claim.priorClaim) {
    const exact = claim.priorClaim.claimId === claim.claimId
      && claim.priorClaim.authorizationId === claim.authorizationId
      && claim.priorClaim.keyPackageId === keyPackage.keyPackageId;
    return exact
      ? { status: "claimed", claimId: claim.claimId, deduplicated: true }
      : reject("ERROR_CODE_REPLAY_DETECTED");
  }
  if (keyPackage.state === "KEY_PACKAGE_STATE_CONSUMED") return reject("ERROR_CODE_KEY_PACKAGE_CONSUMED");
  if (keyPackage.state !== "KEY_PACKAGE_STATE_AVAILABLE") return reject("ERROR_CODE_KEY_PACKAGE_UNAVAILABLE");
  if (keyPackage.notBefore !== keyPackage.issuedAt - 3600) return reject("ERROR_CODE_KEY_PACKAGE_UNAVAILABLE");
  if (keyPackage.expiresAt - keyPackage.notBefore > 7261200) return reject("ERROR_CODE_KEY_PACKAGE_UNAVAILABLE");
  if (now < keyPackage.notBefore || now > keyPackage.expiresAt) return reject("ERROR_CODE_KEY_PACKAGE_UNAVAILABLE");
  return { status: "claimed", claimId: claim.claimId };
}

const transitionKinds = new Set([
  "MEMBERSHIP_CHANGE_KIND_ADD_DEVICE",
  "MEMBERSHIP_CHANGE_KIND_REMOVE_DEVICE",
  "MEMBERSHIP_CHANGE_KIND_REVOKE_DEVICE",
  "MEMBERSHIP_CHANGE_KIND_UPDATE_DEVICE_KEY",
  "MEMBERSHIP_CHANGE_KIND_SELF_UPDATE",
  "MEMBERSHIP_CHANGE_KIND_RECOVERY_KEY_ROTATION",
  "MEMBERSHIP_CHANGE_KIND_REINITIALIZE",
]);

function evaluateMembership(testCase) {
  const { membership, now, changeKind } = testCase;
  if (!transitionKinds.has(changeKind)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
  if (membership.tenantId !== membership.groupTenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
  if (membership.callerDeviceId !== membership.committerDeviceId) return reject("ERROR_CODE_PERMISSION_DENIED");
  if (now > membership.expiresAt) return reject("ERROR_CODE_GRANT_EXPIRED");
  if (!membership.bindingHashValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  const actualBinding = {
    authorizationId: membership.authorizationId,
    tenantId: membership.tenantId,
    groupId: membership.groupId,
    kind: changeKind,
    addKeyPackageIds: [...membership.addKeyPackageIds].sort(),
    removeDeviceIds: [...membership.removeDeviceIds].sort(),
    expectedEpoch: membership.expectedEpoch,
    changeSeq: membership.changeSeq,
    committerDeviceId: membership.committerDeviceId,
    issuedAt: membership.issuedAt,
    expiresAt: membership.expiresAt,
    successorEpoch: membership.successorEpoch,
    policyVersion: membership.policyVersion,
    successorProfile: membership.successorProfile,
    updateDeviceIds: [...membership.updateDeviceIds].sort(),
    successorRecoveryKeyVersion: membership.successorRecoveryKeyVersion,
    successorE2eeGroupId: membership.successorE2eeGroupId,
  };
  const bound = {
    ...membership.authorizationBinding,
    addKeyPackageIds: [...membership.authorizationBinding.addKeyPackageIds].sort(),
    removeDeviceIds: [...membership.authorizationBinding.removeDeviceIds].sort(),
    updateDeviceIds: [...membership.authorizationBinding.updateDeviceIds].sort(),
  };
  if (JSON.stringify(actualBinding) !== JSON.stringify(bound)) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (membership.priorAuthorization) {
    return membership.priorAuthorization.bindingHashValid
      ? { status: "accepted", successorEpoch: membership.successorEpoch, changeSeq: membership.changeSeq, deduplicated: true }
      : reject("ERROR_CODE_REPLAY_DETECTED");
  }
  if (membership.changeSeq !== membership.nextChangeSeq) return reject("ERROR_CODE_REPLAY_DETECTED");
  if (membership.expectedEpoch < membership.currentEpoch) return reject("ERROR_CODE_EPOCH_STALE");
  if (membership.expectedEpoch > membership.currentEpoch || membership.successorEpoch > membership.currentEpoch + 1) {
    return reject("ERROR_CODE_EPOCH_AHEAD");
  }
  if (membership.successorEpoch !== membership.expectedEpoch + 1) return reject("ERROR_CODE_EPOCH_STALE");
  const noAdds = membership.addKeyPackageIds.length === 0;
  const noRemoves = membership.removeDeviceIds.length === 0;
  const noUpdates = membership.updateDeviceIds.length === 0;
  const noRecoveryRotation = membership.successorRecoveryKeyVersion === 0;
  const noReinitialization = !membership.successorProfile && !membership.successorE2eeGroupId;
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_ADD_DEVICE") {
    if (membership.addKeyPackageIds.length === 0) return reject("ERROR_CODE_KEY_PACKAGE_UNAVAILABLE");
    if (!noRemoves || !noUpdates || !noRecoveryRotation || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_REMOVE_DEVICE") {
    if (membership.removeDeviceIds.length === 0 || !noAdds || !noUpdates || !noRecoveryRotation || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_REVOKE_DEVICE") {
    if ((noRemoves && noUpdates) || !noAdds || !noRecoveryRotation || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
    return { status: "accepted", successorEpoch: membership.successorEpoch, changeSeq: membership.changeSeq, invalidatesUnusedKeyPackages: true };
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_UPDATE_DEVICE_KEY" || changeKind === "MEMBERSHIP_CHANGE_KIND_SELF_UPDATE") {
    if (membership.updateDeviceIds.length === 0 || !noAdds || !noRemoves || !noRecoveryRotation || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_RECOVERY_KEY_ROTATION") {
    if (!noAdds || !noRemoves || !noUpdates || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (membership.successorRecoveryKeyVersion <= membership.currentRecoveryKeyVersion) return reject("ERROR_CODE_REPLAY_DETECTED");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_REINITIALIZE") {
    if (!noAdds || !noRemoves || !noUpdates || !noRecoveryRotation) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (membership.successorGroup.tenantId !== membership.groupTenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
    if (membership.successorGroup.parent !== membership.groupParent
      || membership.successorGroup.groupId !== membership.successorE2eeGroupId
      || membership.currentSuccessorE2eeGroupId !== membership.successorE2eeGroupId
      || membership.successorGroup.predecessorGroupId !== membership.groupId) return reject("ERROR_CODE_GROUP_MISMATCH");
    if (!membership.successorProfile
      || membership.successorProfile === membership.groupProfile
      || membership.successorGroup.profile !== membership.successorProfile) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
    if (membership.successorGroup.generation !== membership.groupGeneration + 1) {
      return reject(membership.successorGroup.generation > membership.groupGeneration + 1 ? "ERROR_CODE_EPOCH_AHEAD" : "ERROR_CODE_EPOCH_STALE");
    }
    return { status: "accepted", successorEpoch: membership.successorEpoch, changeSeq: membership.changeSeq, successorGenerationRequired: true };
  }
  return { status: "accepted", successorEpoch: membership.successorEpoch, changeSeq: membership.changeSeq };
}

function evaluateRekeyOutbox(testCase) {
  assert(testCase.membership.state === "E2EE_GROUP_STATE_REKEY_REQUIRED", `${testCase.id}: fixture must exercise rekey_required`);
  return { status: "pending-local-outbox", errorCode: "ERROR_CODE_REKEY_REQUIRED", serverWrite: false };
}

function evaluateWire(testCase) {
  const { wire } = testCase;
  const resultSuffix = { serverParsedMlsData: false };
  const allowedMessageTypes = new Set([
    "MLS_WIRE_MESSAGE_TYPE_COMMIT",
    "MLS_WIRE_MESSAGE_TYPE_WELCOME",
    "MLS_WIRE_MESSAGE_TYPE_PROPOSAL",
    "MLS_WIRE_MESSAGE_TYPE_GROUP_INFO",
  ]);
  if (!allowedMessageTypes.has(wire.messageType)) return reject("ERROR_CODE_ENVELOPE_VERSION_UNSUPPORTED", resultSuffix);
  if (wire.tenantId !== wire.outerTenantId) return reject("ERROR_CODE_TENANT_MISMATCH", resultSuffix);
  if (wire.groupId !== wire.outerGroupId) return reject("ERROR_CODE_GROUP_MISMATCH", resultSuffix);
  if (wire.senderDeviceId !== wire.outerSenderDeviceId) return reject("ERROR_CODE_PERMISSION_DENIED", resultSuffix);
  if (!wire.opaqueBytesUntouched) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT", resultSuffix);
  const handshake = wire.messageType === "MLS_WIRE_MESSAGE_TYPE_COMMIT" || wire.messageType === "MLS_WIRE_MESSAGE_TYPE_PROPOSAL";
  if (handshake && wire.wireFormat !== "MLS_WIRE_FORMAT_PRIVATE_MESSAGE") {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT", resultSuffix);
  }
  if (!handshake && wire.wireFormat !== "MLS_WIRE_FORMAT_INDEPENDENT") {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT", resultSuffix);
  }
  if (wire.messageType === "MLS_WIRE_MESSAGE_TYPE_COMMIT") {
    if (!wire.authorizationId || wire.successorEpoch !== wire.epoch + 1 || wire.changeSeq === 0) return reject("ERROR_CODE_EPOCH_STALE", resultSuffix);
  } else if (wire.successorEpoch !== 0 || wire.authorizationId) {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT", resultSuffix);
  }
  if (wire.messageType === "MLS_WIRE_MESSAGE_TYPE_WELCOME"
    && (wire.targetDeviceIds.length === 0 || !wire.relatedHandshakeId || wire.changeSeq === 0)) {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT", resultSuffix);
  }
  return { status: "accepted", ...resultSuffix };
}

function evaluateHistory(testCase) {
  const { history, now } = testCase;
  if (!history.sourceEligible) return { status: "unavailable", grants: 0, automaticPrejoinAccess: false };
  if (history.tenantId !== history.groupTenantId || history.tenantId !== history.targetTenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
  if (!history.targetAuthorized || !history.signatureValid) return reject("ERROR_CODE_HISTORY_SHARING_DENIED");
  if (history.firstEpoch !== history.requestedFirstEpoch
    || history.lastEpoch !== history.requestedLastEpoch
    || history.policyVersion !== history.requestPolicyVersion) return reject("ERROR_CODE_HISTORY_SHARING_DENIED");
  if (history.firstEpoch < history.retentionFirstEpoch || history.lastEpoch > history.retentionLastEpoch) {
    return reject("ERROR_CODE_RETENTION_EXPIRED");
  }
  if (history.firstEpoch > history.lastEpoch) return reject("ERROR_CODE_EPOCH_UNAVAILABLE");
  if (now > Math.min(history.requestExpiresAt, history.grantExpiresAt, history.retentionExpiresAt)) return reject("ERROR_CODE_GRANT_EXPIRED");
  return { status: "accepted", firstEpoch: history.firstEpoch, lastEpoch: history.lastEpoch };
}

function evaluateRecovery(testCase) {
  const { recovery, now } = testCase;
  if (!recovery.executionId) return reject("ERROR_CODE_INVALID_STATE_TRANSITION");
  if (!recovery.requestId) return reject("ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT");
  const exactExecutionRetry = recovery.state === "RECOVERY_CASE_STATE_EXECUTED"
    && Boolean(recovery.priorExecutionId)
    && recovery.priorExecutionId === recovery.executionId;
  if (recovery.state === "RECOVERY_CASE_STATE_EXECUTED" && recovery.priorExecutionId !== recovery.executionId) {
    return recovery.priorExecutionId
      ? reject("ERROR_CODE_REPLAY_DETECTED")
      : reject("ERROR_CODE_INVALID_STATE_TRANSITION");
  }
  if (recovery.state !== "RECOVERY_CASE_STATE_APPROVED" && !exactExecutionRetry) {
    return reject("ERROR_CODE_INVALID_STATE_TRANSITION");
  }
  if (recovery.state === "RECOVERY_CASE_STATE_APPROVED" && recovery.priorExecutionId) {
    return reject("ERROR_CODE_INVALID_STATE_TRANSITION");
  }
  if (recovery.tenantId !== recovery.recipientTenantId || recovery.envelope.tenantId !== recovery.tenantId) {
    return reject("ERROR_CODE_TENANT_MISMATCH");
  }
  if (!recovery.recipientAuthorized) {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (now > recovery.expiresAt) return reject("ERROR_CODE_RECOVERY_CASE_EXPIRED");
  const canonicalScopeIds = recovery.scopes.map((scope) => scope.groupId);
  const sortedScopeIds = [...canonicalScopeIds].sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)));
  if (canonicalScopeIds.length === 0
    || new Set(canonicalScopeIds).size !== canonicalScopeIds.length
    || JSON.stringify(canonicalScopeIds) !== JSON.stringify(sortedScopeIds)) {
    return reject("ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT");
  }
  const sharedScope = recovery.scopes[0];
  if (recovery.scopes.some((scope) => scope.firstEpoch !== sharedScope.firstEpoch
    || scope.lastEpoch !== sharedScope.lastEpoch
    || scope.startTime !== sharedScope.startTime
    || scope.endTime !== sharedScope.endTime)) {
    return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  }
  const legacyScopeProjectionMatches = JSON.stringify(recovery.legacyGroupIds) === JSON.stringify(canonicalScopeIds)
    && recovery.legacyFirstEpoch === sharedScope.firstEpoch
    && recovery.legacyLastEpoch === sharedScope.lastEpoch
    && recovery.legacyStartTime === sharedScope.startTime
    && recovery.legacyEndTime === sharedScope.endTime;
  if (!legacyScopeProjectionMatches) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  const caseApprovalProjection = {
    recoveryCaseId: recovery.caseId,
    requestId: recovery.requestId,
    tenantId: recovery.tenantId,
    requester: {
      actorId: recovery.requester.actorId,
      actorType: recovery.requester.actorType,
    },
    reasonDigest: recovery.reasonDigest,
    scopes: recovery.scopes.map((scope) => ({
      groupId: scope.groupId,
      firstEpoch: scope.firstEpoch,
      lastEpoch: scope.lastEpoch,
      startTime: scope.startTime,
      endTime: scope.endTime,
    })),
    recipientDeviceId: recovery.recipientDeviceId,
    requiredApprovals: recovery.requiredApprovals,
    expiresAt: recovery.expiresAt,
    policyVersion: recovery.policyVersion,
  };
  const expectedCaseBindingHash = sha256(Buffer.from(JSON.stringify(caseApprovalProjection)));
  if (recovery.approverActorIds.length < Math.max(2, recovery.requiredApprovals)
    || recovery.approvalIds.length !== recovery.approverActorIds.length
    || recovery.decisionIds.length !== recovery.approverActorIds.length
    || recovery.approvalDecidedAts.length !== recovery.approverActorIds.length
    || recovery.approvalPolicyVersions.length !== recovery.approverActorIds.length
    || recovery.approvalCaseBindingHashes.length !== recovery.approverActorIds.length
    || recovery.decisionSignatures.length !== recovery.approverActorIds.length
    || recovery.approvalDecisions.length !== recovery.approverActorIds.length
    || recovery.approvalNotes.length !== recovery.approverActorIds.length
    || recovery.approverDeviceIds.length !== recovery.approverActorIds.length
    || recovery.approvalSessionDeviceIds.length !== recovery.approverActorIds.length
    || new Set(recovery.approverActorIds).size !== recovery.approverActorIds.length
    || new Set(recovery.decisionIds).size !== recovery.decisionIds.length
    || recovery.approverActorIds.includes(recovery.requester.actorId)
    || recovery.approvalPolicyVersions.some((version) => version !== recovery.policyVersion)
    || recovery.approvalDecisions.some((approved) => !approved)
    || recovery.approvalCaseBindingHashes.some((binding) => binding !== expectedCaseBindingHash)
    || recovery.approverActorIds.some((_, index) => {
      const decisionProjection = {
        caseBindingHash: expectedCaseBindingHash,
        recoveryCaseId: recovery.caseId,
        approved: recovery.approvalDecisions[index],
        note: {
          ciphertext: recovery.approvalNotes[index].ciphertext,
          e2eeGroupId: recovery.approvalNotes[index].e2eeGroupId,
          epoch: recovery.approvalNotes[index].epoch,
          cryptoProfile: recovery.approvalNotes[index].cryptoProfile,
          envelopeVersion: recovery.approvalNotes[index].envelopeVersion,
        },
        decisionId: recovery.decisionIds[index],
        approverDeviceId: recovery.approverDeviceIds[index],
      };
      return recovery.decisionSignatures[index] !== sha256(Buffer.from(JSON.stringify(decisionProjection)));
    })) {
    return reject("ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT");
  }
  if (recovery.approverDeviceIds.some((deviceId, index) => deviceId !== recovery.approvalSessionDeviceIds[index])) {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  const executionScope = recovery.scopes.find((scope) => scope.groupId === recovery.envelope.groupId);
  if (!executionScope) return reject("ERROR_CODE_GROUP_MISMATCH");
  if (recovery.envelope.epoch < executionScope.firstEpoch || recovery.envelope.epoch > executionScope.lastEpoch) {
    return reject("ERROR_CODE_EPOCH_UNAVAILABLE");
  }
  if (recovery.envelope.createdAt < executionScope.startTime || recovery.envelope.createdAt >= executionScope.endTime) {
    return reject("ERROR_CODE_EPOCH_UNAVAILABLE");
  }
  if (!recovery.envelope.scopeHash || !recovery.envelope.scopeBindingHash) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  if (recovery.envelope.scopeBindingHash !== `${recovery.envelope.bindingHash}+${recovery.envelope.scopeHash}`) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (!recovery.envelope.recoveryRecipientPresent) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  if (now > recovery.delivery.deliveryExpiresAt || recovery.delivery.deliveryExpiresAt > recovery.expiresAt) return reject("ERROR_CODE_RECOVERY_CASE_EXPIRED");
  const deliveryBindingProjection = {
    tenantId: recovery.delivery.tenantId,
    e2eeGroupId: recovery.delivery.groupId,
    epoch: recovery.delivery.epoch,
    cryptoProfile: recovery.delivery.cryptoProfile,
    recoveryKeyVersion: recovery.delivery.recoveryKeyVersion,
    wrappedMaterial: recovery.delivery.wrappedMaterial,
    envelopeVersion: recovery.delivery.envelopeVersion,
    recoveryRecipientPresent: recovery.delivery.recoveryRecipientPresent,
    recoveryKeyId: recovery.delivery.recoveryKeyId,
    bindingHash: recovery.delivery.bindingHash,
    scopeHash: recovery.delivery.scopeHash,
    scopeBindingHash: recovery.delivery.scopeBindingHash,
    recoveryCaseId: recovery.delivery.recoveryCaseId,
    recoveryRecipientDeviceId: recovery.delivery.recipientDeviceId,
    deliveryPolicyVersion: recovery.delivery.deliveryPolicyVersion,
    deliveryExpiresAt: recovery.delivery.deliveryExpiresAt,
  };
  if (recovery.delivery.deliveryBindingHash !== sha256(Buffer.from(JSON.stringify(deliveryBindingProjection)))) {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  const projectionMatches = recovery.delivery.recoveryCaseId === recovery.caseId
    && recovery.delivery.tenantId === recovery.envelope.tenantId
    && recovery.delivery.groupId === recovery.envelope.groupId
    && recovery.delivery.epoch === recovery.envelope.epoch
    && recovery.delivery.recipientDeviceId === recovery.recipientDeviceId
    && recovery.delivery.deliveryPolicyVersion === recovery.policyVersion
    && recovery.delivery.cryptoProfile === recovery.envelope.cryptoProfile
    && recovery.delivery.recoveryKeyVersion === recovery.envelope.recoveryKeyVersion
    && recovery.delivery.recoveryKeyId === recovery.envelope.recoveryKeyId
    && recovery.delivery.bindingHash === recovery.envelope.bindingHash
    && recovery.delivery.scopeHash === recovery.envelope.scopeHash
    && recovery.delivery.scopeBindingHash === recovery.envelope.scopeBindingHash;
  if (!projectionMatches) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  return {
    status: "delivered",
    recipientDeviceId: recovery.recipientDeviceId,
    deliveries: 1,
    nMinusOneResult: true,
    ...(exactExecutionRetry ? { deduplicated: true } : {}),
  };
}

function evaluateLegacyRecoveryExecute(testCase) {
  const { legacyRecoveryExecute: legacy } = testCase;
  assert(!legacy.scopeHashPresent && !legacy.scopeBindingHashPresent && !legacy.deliveryMetadataPresent,
    `${testCase.id}: fixture must model an N-1 envelope missing current execution metadata`);
  return reject("ERROR_CODE_RECOVERY_UNAVAILABLE", {
    bytesDecoded: legacy.bytesDecoded,
    bytesPreserved: legacy.bytesPreserved,
    auditVisible: legacy.auditVisible,
    objectOrTimeScopeInferred: false,
  });
}

function evaluateLegacyRecoveryCreate(testCase) {
  const legacy = testCase.legacyRecoveryCreate;
  const groupIds = [...new Set(legacy.groupIds)]
    .sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)));
  if (groupIds.length === 0 || groupIds.length !== legacy.groupIds.length) {
    return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  }
  return {
    status: "accepted",
    normalizedScopes: groupIds.map((groupId) => ({
      groupId,
      firstEpoch: legacy.firstEpoch,
      lastEpoch: legacy.lastEpoch,
      startTime: legacy.startTime,
      endTime: legacy.endTime,
    })),
    unioned: false,
  };
}

function decodeVarint(bytes, start) {
  let value = 0n;
  let shift = 0n;
  let offset = start;
  while (offset < bytes.length && shift <= 63n) {
    const byte = bytes[offset];
    value |= BigInt(byte & 0x7f) << shift;
    offset += 1;
    if ((byte & 0x80) === 0) return { value, next: offset };
    shift += 7n;
  }
  throw new Error(`invalid varint at byte ${start}`);
}

function parseWire(bytes) {
  const fields = [];
  let offset = 0;
  while (offset < bytes.length) {
    const start = offset;
    const tag = decodeVarint(bytes, offset);
    offset = tag.next;
    const number = Number(tag.value >> 3n);
    const wireType = Number(tag.value & 7n);
    assert(number > 0, `invalid field number at byte ${start}`);
    if (wireType === 0) offset = decodeVarint(bytes, offset).next;
    else if (wireType === 1) offset += 8;
    else if (wireType === 2) {
      const length = decodeVarint(bytes, offset);
      offset = length.next + Number(length.value);
    } else if (wireType === 5) offset += 4;
    else throw new Error(`unsupported wire type ${wireType} for field ${number}`);
    assert(offset <= bytes.length, `truncated field ${number}`);
    fields.push({ number, raw: bytes.subarray(start, offset) });
  }
  return fields;
}

function evaluateCompatibility(testCase) {
  const fixturePath = resolve(fixtureRoot, testCase.fixture);
  const fixtureRelative = relative(repositoryRoot, fixturePath);
  assert(fixtureRelative !== "" && !fixtureRelative.startsWith("..") && !isAbsolute(fixtureRelative), `${testCase.id}: fixture escaped repository`);
  const hex = readFileSync(fixturePath, "utf8").trim();
  assert(/^(?:[0-9a-f]{2})+$/u.test(hex), `${testCase.id}: Golden Frame must be canonical lowercase hex`);
  const bytes = Buffer.from(hex, "hex");
  const fields = parseWire(bytes);
  assert(fields.filter((field) => field.number === testCase.fieldNumber).length === 1, `${testCase.id}: unknown field ${testCase.fieldNumber} missing`);
  assert(Buffer.concat(fields.map((field) => field.raw)).equals(bytes), `${testCase.id}: static parser seam changed fixture bytes`);
  // This local structural seam is not generated-adapter or five-language
  // evidence. It proves only that the committed canary exists and that the
  // local field slicer can retain its input bytes for later Integration tests.
  assert(!fields.some((field) => field.number === 11), `${testCase.id}: N-1 frame unexpectedly contains T019 scope_hash`);
  assert(!fields.some((field) => field.number === 12), `${testCase.id}: N-1 frame unexpectedly contains T019 scope_binding_hash`);
  for (const fieldNumber of [13, 14, 15, 16, 17]) {
    assert(!fields.some((field) => field.number === fieldNumber), `${testCase.id}: stored frame unexpectedly contains delivery-only field ${fieldNumber}`);
  }
  return { wireCanaryPresent: true, staticPreservationSeamPresent: true, currentScopedExecution: "ERROR_CODE_RECOVERY_UNAVAILABLE" };
}

function evaluate(testCase) {
  if (testCase.kind === "credential") return evaluateCredential(testCase);
  if (testCase.kind === "key-package") return evaluateKeyPackage(testCase);
  if (testCase.kind === "membership") return evaluateMembership(testCase);
  if (testCase.kind === "rekey-outbox") return evaluateRekeyOutbox(testCase);
  if (testCase.kind === "wire") return evaluateWire(testCase);
  if (testCase.kind === "history") return evaluateHistory(testCase);
  if (testCase.kind === "recovery") return evaluateRecovery(testCase);
  if (testCase.kind === "legacy-recovery-execute") return evaluateLegacyRecoveryExecute(testCase);
  if (testCase.kind === "legacy-recovery-create") return evaluateLegacyRecoveryCreate(testCase);
  if (testCase.kind === "compatibility") return evaluateCompatibility(testCase);
  throw new Error(`${testCase.id}: unknown case kind ${testCase.kind}`);
}

assert(manifest.schemaVersion === 1, "fixture manifest schemaVersion must be 1");
assert(manifest.classification === "synthetic-crypto-contract-behavior-no-secrets", "fixture classification must remain synthetic and secret-free");
assert(JSON.stringify(manifest.reviewers) === JSON.stringify(["Contracts", "Architecture", "Security"]), "limited-input review ownership must remain explicit");
assert(manifest.provenance.issue === 37 && manifest.provenance.generator === "none", "fixture provenance must remain bound to T019");
assert(manifest.acceptance.physicalDevices === "NOT RUN", "T019 must not claim physical-device evidence");
assert(manifest.acceptance.productionOpenMlsAdmission === "NOT ESTABLISHED", "T019 must not claim production OpenMLS admission");
assert(manifest.acceptance.generatedFiveLanguageNMinusOne === "PENDING_INTEGRATION", "five-language N-1 evidence must remain an Integration-owned pending gate");
for (const [name, value] of Object.entries(manifest.acceptance.localContractScenarioCoverage)) assert(value === true, `${name} local scenario coverage must remain true`);
assert(manifest.nMinusOne.generatedFiveLanguageNMinusOne === "PENDING_INTEGRATION", "manifest must not promote the local wire seam to generated evidence");
assert(manifest.nMinusOne.legacyKeyPackageMigration.includes("PENDING_IMPLEMENTATION"), "N-1 KeyPackage migration must not claim client crypto evidence");
assert(sha256(scenarioBytes) === manifest.source.sha256, "scenario source SHA-256 mismatch");
assert(scenarios.schemaVersion === manifest.schemaVersion && scenarios.classification === manifest.classification, "scenario metadata differs from manifest");
for (const contract of manifest.contracts) assert(statSync(join(repositoryRoot, "proto", contract)).isFile(), `missing contract ${contract}`);

const crypto = read("proto/threadline/crypto/v1/crypto.proto");
const recovery = read("proto/threadline/crypto/v1/recovery.proto");
const services = read("proto/threadline/crypto/v1/key_service.proto");
const errors = read("proto/threadline/type/v1/error.proto");

for (const assertion of [
  [crypto, /uint32\s+credential_format_version\s*=\s*11;/u, "Device Credential format version"],
  [crypto, /CredentialIssuerKind\s+issuer_kind\s*=\s*12;/u, "Device Credential issuer"],
  [crypto, /fields 1-8 followed by fields 10-12,[\s\S]*Field 9 is the[\s\S]*signature itself/u, "non-self-referential Device Credential signed projection"],
  [crypto, /string\s+tenant_id\s*=\s*8;/u, "KeyPackage tenant binding"],
  [crypto, /google\.protobuf\.Timestamp\s+not_before\s*=\s*10;/u, "KeyPackage not-before"],
  [crypto, /google\.protobuf\.Timestamp\s+issued_at\s*=\s*14;/u, "KeyPackage issuance time"],
  [crypto, /bytes\s+publication_binding_hash\s*=\s*15;/u, "KeyPackage immutable publication binding"],
  [crypto, /bytes\s+publication_signature\s*=\s*16;/u, "KeyPackage Device publication signature"],
  [crypto, /fields 1-6, 8-10 and 14[\s\S]*fields 7 and 11-13 are intentionally excluded/u, "KeyPackage publication signed projection"],
  [crypto, /KeyPackageState\s+state\s*=\s*11;/u, "KeyPackage terminal state"],
  [crypto, /uint64\s+generation\s*=\s*10;/u, "Group generation"],
  [crypto, /uint64\s+successor_epoch\s*=\s*12;/u, "membership successor Epoch"],
  [crypto, /repeated\s+string\s+update_device_ids\s*=\s*16;/u, "rotation and revocation targets"],
  [crypto, /uint32\s+successor_recovery_key_version\s*=\s*17;/u, "recovery-key rotation target"],
  [crypto, /string\s+successor_e2ee_group_id\s*=\s*18;/u, "reinitialization successor Group"],
  [crypto, /fields 1-13[\s\S]*fields 15-18[\s\S]*policy_version[\s\S]*successor_crypto_profile[\s\S]*update_device_ids[\s\S]*successor_recovery_key_version[\s\S]*successor_e2ee_group_id/u, "complete membership authorization binding projection"],
  [crypto, /MLS_WIRE_FORMAT_PRIVATE_MESSAGE\s*=\s*1;/u, "PrivateMessage wire format"],
  [crypto, /string\s+handshake_id\s*=\s*12;/u, "handshake replay identity"],
  [crypto, /string\s+related_handshake_id\s*=\s*13;/u, "Welcome-to-Commit correlation"],
  [recovery, /message\s+HistorySharingRequest\s*\{/u, "bounded history request"],
  [recovery, /repeated\s+RecoveryScope\s+scopes\s*=\s*16;/u, "explicit recovery scopes"],
  [recovery, /bytes\s+case_binding_hash\s*=\s*8;/u, "Recovery Approval Case binding"],
  [recovery, /bytes\s+decision_signature\s*=\s*9;/u, "Recovery Approval decision signature"],
  [recovery, /string\s+approver_device_id\s*=\s*10;/u, "Recovery Approval signing Device"],
  [recovery, /case_binding_hash[\s\S]*DecideRecoveryCaseRequest fields 1-4 and 7[\s\S]*Service-generated approval_id[\s\S]*excluded/u, "client-generatable Recovery decision signature projection"],
  [recovery, /requester ActorRef identity[\s\S]*canonical `scopes`[\s\S]*Legacy parallel scope[\s\S]*not inputs to this current hash/u, "authoritative Recovery Case approval projection"],
  [recovery, /complete encrypted note \(ciphertext, e2ee_group_id, epoch,[\s\S]*crypto_profile and envelope_version\)/u, "complete encrypted approval-note signature binding"],
  [recovery, /ascending bytewise e2ee_group_id order[\s\S]*same shared inclusive Epoch bounds and half-open TimeRange[\s\S]*exactly[\s\S]*double-write sorted Group IDs/u, "canonical shared-bound Recovery scopes"],
  [recovery, /bytes\s+scope_hash\s*=\s*11;/u, "Recovery Envelope scope hash"],
  [recovery, /bytes\s+scope_binding_hash\s*=\s*12;/u, "Recovery Envelope scope-aware binding"],
  [recovery, /string\s+recovery_case_id\s*=\s*13;/u, "RecoveryEnvelope delivery Case binding"],
  [recovery, /string\s+recovery_recipient_device_id\s*=\s*14;/u, "RecoveryEnvelope delivery recipient"],
  [recovery, /string\s+delivery_policy_version\s*=\s*15;/u, "RecoveryEnvelope delivery policy"],
  [recovery, /google\.protobuf\.Timestamp\s+delivery_expires_at\s*=\s*16;/u, "RecoveryEnvelope delivery expiry"],
  [recovery, /bytes\s+delivery_binding_hash\s*=\s*17;/u, "RecoveryEnvelope current delivery binding"],
  [recovery, /fields 1-16[\s\S]*Field 17 is the digest itself and is excluded/u, "complete RecoveryEnvelope delivery projection"],
  [services, /MessageService\.SendEvent/u, "single durable MLS sequencing path"],
  [services, /string\s+claim_id\s*=\s*4;/u, "atomic claim replay identity"],
  [services, /message\s+DecideRecoveryCaseRequest\s*\{[\s\S]*bytes\s+case_binding_hash\s*=\s*5;[\s\S]*bytes\s+decision_signature\s*=\s*6;[\s\S]*string\s+approver_device_id\s*=\s*7;/u, "approver-submitted Recovery Case binding and Device signature"],
  [services, /message\s+ExecuteRecoveryCaseRequest\s*\{[\s\S]*Empty is invalid[\s\S]*string\s+execution_id\s*=\s*2;/u, "non-empty Recovery execution idempotency key"],
  [services, /Current writers exactly double-write them to fields 2-5[\s\S]*N-1 request is[\s\S]*normalized into one RecoveryScope per sorted Group/u, "v1/N-1 Recovery scope double-write normalization"],
  [services, /repeated\s+RecoveryEnvelope\s+delivered_envelopes\s*=\s*2;/u, "single N-1/current recovery delivery surface"],
]) assert(assertion[1].test(assertion[0]), `contract is missing ${assertion[2]}`);
assert(!/message\s+RecoveryDeliveryEnvelope\s*\{/u.test(recovery), "T019 must not add a second recovery delivery message");
assert(!/\bdeliveries\s*=\s*3;/u.test(services), "ExecuteRecoveryCaseResponse must not add a second delivery field");

assert(crypto.includes("7,261,200 seconds"), "exact maximum KeyPackage lifetime must remain documented");
assert(crypto.includes("issued_at - 3600 seconds"), "exact issuance-time not-before skew must remain documented");
assert(!/openmls[_ .-]*0\.[0-9]/iu.test(`${crypto}\n${recovery}\n${services}`), "library release numbers must not enter cross-platform Proto");
assert(!/bytes\s+(?:group_key|epoch_key|ratchet_secret|recovery_private_key|plaintext)\b/iu.test(`${crypto}\n${recovery}\n${services}`), "server-visible contract must not expose forbidden secret/plaintext fields");
assert(!services.includes("threadline/identity/v1/device.proto"), "T019 must reuse, not import or duplicate, the Identity-owned Device resource");
for (const code of [
  "ERROR_CODE_PERMISSION_DENIED", "ERROR_CODE_TENANT_MISMATCH", "ERROR_CODE_GROUP_MISMATCH",
  "ERROR_CODE_HISTORY_SHARING_DENIED", "ERROR_CODE_GRANT_EXPIRED", "ERROR_CODE_DEVICE_NOT_ENROLLED",
  "ERROR_CODE_DEVICE_REVOKED", "ERROR_CODE_KEY_PACKAGE_UNAVAILABLE", "ERROR_CODE_KEY_PACKAGE_CONSUMED",
  "ERROR_CODE_REKEY_REQUIRED", "ERROR_CODE_EPOCH_STALE", "ERROR_CODE_EPOCH_UNAVAILABLE",
  "ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED", "ERROR_CODE_EPOCH_AHEAD", "ERROR_CODE_CIPHERTEXT_CORRUPT",
  "ERROR_CODE_REPLAY_DETECTED", "ERROR_CODE_ENVELOPE_VERSION_UNSUPPORTED",
  "ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT", "ERROR_CODE_RECOVERY_CASE_EXPIRED",
  "ERROR_CODE_RECOVERY_UNAVAILABLE",
]) assert(errors.includes(code), `stable shared error vocabulary is missing ${code}`);

const rawCases = new Map(scenarios.cases.map((testCase) => [testCase.id, testCase]));
assert(rawCases.size === scenarios.cases.length, "scenario IDs must be unique");
assert(JSON.stringify([...rawCases.keys()]) === JSON.stringify(manifest.requiredCases), "scenario set/order must exactly match requiredCases");
for (const id of manifest.requiredCases) {
  const raw = rawCases.get(id);
  const testCase = merge(scenarios.defaults, raw);
  testCase.id = raw.id;
  testCase.kind = raw.kind;
  testCase.expected = raw.expected;
  const actual = evaluate(testCase);
  assert(JSON.stringify(actual) === JSON.stringify(testCase.expected), `${id}: expected ${JSON.stringify(testCase.expected)}, got ${JSON.stringify(actual)}`);
  const visible = JSON.stringify({ testCase, actual });
  assert(!visible.includes(scenarios.forbiddenPlaintextCanary), `${id}: plaintext canary crossed the contract boundary`);
}

const goldenHex = readFileSync(join(repositoryRoot, manifest.goldenReference.file), "utf8").trim();
assert(sha256(Buffer.from(goldenHex, "hex")) === manifest.goldenReference.decodedSha256, "referenced T014 RecoveryEnvelope digest changed");
assert(manifest.goldenReference.unknownFieldNumber === 50000, "RecoveryEnvelope unknown field must remain 50000");

console.log(`Verified ${manifest.requiredCases.length} synthetic T019 crypto/recovery contract scenarios; physical devices NOT RUN; production OpenMLS admission NOT ESTABLISHED.`);
