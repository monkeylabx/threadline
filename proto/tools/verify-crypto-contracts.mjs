import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixtureRoot = join(repositoryRoot, "test", "fixtures", "proto", "crypto");
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "manifest.json"), "utf8"));
const scenarioBytes = readFileSync(join(fixtureRoot, manifest.source.file));
const scenarios = JSON.parse(scenarioBytes.toString("utf8"));
const transcriptBytesFixture = readFileSync(join(fixtureRoot, manifest.transcripts.file));
const transcriptVectors = JSON.parse(transcriptBytesFixture.toString("utf8"));

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

// RFC 8785 JCS subset used by these fixtures. Protocol uint64 values are
// decimal strings before reaching this function; timestamps are normalized
// RFC 3339 UTC strings and bytes are lowercase hex strings.
function canonicalize(value) {
  if (value === null || typeof value === "boolean" || typeof value === "string") return JSON.stringify(value);
  if (typeof value === "number") {
    assert(Number.isSafeInteger(value), "canonical transcript numbers must be safe integers");
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) return `[${value.map(canonicalize).join(",")}]`;
  assert(typeof value === "object", `unsupported canonical transcript value ${typeof value}`);
  return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${canonicalize(value[key])}`).join(",")}}`;
}

const transcriptPrefix = "threadline.crypto.transcript/v1/";

function canonicalTranscript(domain, payload) {
  return Buffer.from(`${transcriptPrefix}${domain}\n${canonicalize(payload)}`, "utf8");
}

function transcriptHash(domain, payload) {
  return sha256(canonicalTranscript(domain, payload));
}

function canonicalEqual(left, right) {
  return canonicalize(left) === canonicalize(right);
}

function uint64(value) {
  assert(Number.isSafeInteger(value) && value >= 0, "fixture uint64 must be a non-negative safe integer");
  return String(value);
}

function timestamp(value) {
  assert(Number.isSafeInteger(value) && value >= 0, "fixture timestamp must be whole Unix seconds");
  return new Date(value * 1000).toISOString().replace(".000Z", "Z");
}

function bytesHex(value) {
  return Buffer.from(value, "utf8").toString("hex");
}

function canonicalProtectedScope(scope) {
  return {
    object: scope.object ? {
      kind: String({
        RECOVERY_OBJECT_KIND_EVENT: 1,
        RECOVERY_OBJECT_KIND_BLOB: 2,
        RECOVERY_OBJECT_KIND_ARTIFACT: 3,
      }[scope.object.kind] ?? 0),
      resourceId: scope.object.resourceId,
    } : null,
    historyEpochRange: scope.historyEpochRange ? {
      groupId: scope.historyEpochRange.groupId,
      firstEpoch: uint64(scope.historyEpochRange.firstEpoch),
      lastEpoch: uint64(scope.historyEpochRange.lastEpoch),
    } : null,
  };
}

function canonicalRecoveryScope(scope) {
  return {
    groupId: scope.groupId,
    firstEpoch: uint64(scope.firstEpoch),
    lastEpoch: uint64(scope.lastEpoch),
    startTime: timestamp(scope.startTime),
    endTime: timestamp(scope.endTime),
    protectedTargets: (scope.targets ?? []).map(canonicalProtectedScope),
  };
}

function protectedScopeSortKey(scope) {
  if (scope.object) {
    const kind = {
      RECOVERY_OBJECT_KIND_EVENT: 1,
      RECOVERY_OBJECT_KIND_BLOB: 2,
      RECOVERY_OBJECT_KIND_ARTIFACT: 3,
    }[scope.object.kind] ?? 0;
    return [0, kind, scope.object.resourceId, 0, 0];
  }
  const range = scope.historyEpochRange;
  return [1, range?.groupId ?? "", range?.firstEpoch ?? 0, range?.lastEpoch ?? 0, 0];
}

function compareProtectedScopes(left, right) {
  const leftKey = protectedScopeSortKey(left);
  const rightKey = protectedScopeSortKey(right);
  for (let index = 0; index < leftKey.length; index += 1) {
    const order = typeof leftKey[index] === "number"
      ? leftKey[index] - rightKey[index]
      : Buffer.compare(Buffer.from(leftKey[index]), Buffer.from(rightKey[index]));
    if (order !== 0) return order;
  }
  return 0;
}

function canonicalProfile(profile) {
  return {
    name: profile.name,
    mlsProtocolVersion: profile.mlsProtocolVersion,
    cipherSuite: String({
      CIPHER_SUITE_MLS_128_DHKEMX25519_AES128GCM_SHA256_ED25519: 1,
    }[profile.cipherSuite] ?? 0),
    messageEnvelopeVersion: profile.messageEnvelopeVersion,
    historyEnvelopeVersion: profile.historyEnvelopeVersion,
    recoveryEnvelopeVersion: profile.recoveryEnvelopeVersion,
  };
}

const issuerWireNumber = {
  CREDENTIAL_ISSUER_KIND_DEVICE_AUTHORITY: 1,
  CREDENTIAL_ISSUER_KIND_EXISTING_AUTHORIZED_DEVICE: 2,
  CREDENTIAL_ISSUER_KIND_ADMIN_EXCEPTION: 3,
};

const membershipKindWireNumber = {
  MEMBERSHIP_CHANGE_KIND_ADD_DEVICE: 1,
  MEMBERSHIP_CHANGE_KIND_REMOVE_DEVICE: 2,
  MEMBERSHIP_CHANGE_KIND_UPDATE_DEVICE_KEY: 3,
  MEMBERSHIP_CHANGE_KIND_SELF_UPDATE: 4,
  MEMBERSHIP_CHANGE_KIND_RECOVERY_KEY_ROTATION: 5,
  MEMBERSHIP_CHANGE_KIND_REINITIALIZE: 6,
  MEMBERSHIP_CHANGE_KIND_REVOKE_DEVICE: 7,
};

const eventCategoryWireNumber = {
  EVENT_CATEGORY_APPLICATION: 1,
  EVENT_CATEGORY_REDACTION: 2,
  EVENT_CATEGORY_MLS_HANDSHAKE: 3,
  EVENT_CATEGORY_MLS_WELCOME: 4,
  EVENT_CATEGORY_MEMBERSHIP: 5,
};

function credentialTranscript(credential) {
  return {
    deviceId: credential.deviceId,
    actorId: credential.actorId,
    tenantId: credential.tenantId,
    identityPublicKeyHex: bytesHex(credential.identityPublicKey),
    credentialVersion: credential.credentialVersion,
    issuedAt: timestamp(credential.issuedAt),
    expiresAt: timestamp(credential.expiresAt),
    cryptoProfile: canonicalProfile(credential.profile),
    approvedBy: credential.approvedBy,
    credentialFormatVersion: credential.credentialFormatVersion,
    issuerKind: String(issuerWireNumber[credential.issuerKind] ?? 0),
  };
}

function publicationTranscript(keyPackage) {
  return {
    keyPackageId: keyPackage.keyPackageId,
    deviceId: keyPackage.deviceId,
    keyPackageDataHex: bytesHex(keyPackage.keyPackageData),
    cryptoProfile: canonicalProfile(keyPackage.profile),
    publishedAt: timestamp(keyPackage.publishedAt),
    expiresAt: timestamp(keyPackage.expiresAt),
    tenantId: keyPackage.tenantId,
    credentialVersion: keyPackage.credentialVersion,
    notBefore: timestamp(keyPackage.notBefore),
    issuedAt: timestamp(keyPackage.issuedAt),
  };
}

function membershipTranscript(source, kind) {
  return {
    authorizationId: source.authorizationId,
    tenantId: source.tenantId,
    groupId: source.groupId,
    kind: String(membershipKindWireNumber[kind] ?? 0),
    addKeyPackageIds: [...source.addKeyPackageIds].sort(),
    removeDeviceIds: [...source.removeDeviceIds].sort(),
    expectedEpoch: uint64(source.expectedEpoch),
    changeSeq: uint64(source.changeSeq),
    committerDeviceId: source.committerDeviceId,
    issuedAt: timestamp(source.issuedAt),
    expiresAt: timestamp(source.expiresAt),
    successorEpoch: uint64(source.successorEpoch),
    policyVersion: source.policyVersion,
    successorProfile: source.successorProfile && typeof source.successorProfile === "object"
      ? canonicalProfile(source.successorProfile)
      : null,
    updateDeviceIds: [...source.updateDeviceIds].sort(),
    successorRecoveryKeyVersion: source.successorRecoveryKeyVersion,
    successorE2eeGroupId: source.successorE2eeGroupId,
  };
}

function historyTranscript(history) {
  return {
    grantId: history.grantId,
    e2eeGroupId: history.groupId,
    sourceDeviceId: history.sourceDeviceId,
    targetDeviceId: history.targetDeviceId,
    firstEpoch: uint64(history.firstEpoch),
    lastEpoch: uint64(history.lastEpoch),
    wrappedHistoryKeysHex: bytesHex(history.wrappedHistoryKeys),
    cryptoProfile: canonicalProfile(history.profile),
    policyVersion: history.policyVersion,
    createdAt: timestamp(history.createdAt),
    retentionExpiresAt: timestamp(history.retentionExpiresAt),
    tenantId: history.tenantId,
    requestId: history.requestId,
    expiresAt: timestamp(history.grantExpiresAt),
  };
}

function recoveryCaseTranscript(recovery) {
  return {
    recoveryCaseId: recovery.caseId,
    requestId: recovery.requestId,
    tenantId: recovery.tenantId,
    requester: {
      actorId: recovery.requester.actorId,
      actorType: String({ ACTOR_TYPE_HUMAN: 1, ACTOR_TYPE_AGENT: 2, ACTOR_TYPE_SERVICE: 3 }[recovery.requester.actorType] ?? 0),
    },
    reasonDigestHex: bytesHex(recovery.reasonDigest),
    legacyE2eeGroupIds: [...recovery.legacyGroupIds],
    legacyFirstEpoch: uint64(recovery.legacyFirstEpoch),
    legacyLastEpoch: uint64(recovery.legacyLastEpoch),
    legacyTimeRange: recovery.legacyStartTime === null && recovery.legacyEndTime === null
      ? null
      : {
        startTime: timestamp(recovery.legacyStartTime),
        endTime: timestamp(recovery.legacyEndTime),
      },
    scopes: recovery.scopes.map(canonicalRecoveryScope),
    recipientDeviceId: recovery.recipientDeviceId,
    requiredApprovals: recovery.requiredApprovals,
    expiresAt: timestamp(recovery.expiresAt),
    policyVersion: recovery.policyVersion,
  };
}

function recoveryDecisionTranscript(recovery, index, caseBindingHash) {
  const note = recovery.approvalNotes[index];
  return {
    caseBindingHashHex: caseBindingHash,
    recoveryCaseId: recovery.caseId,
    approved: recovery.approvalDecisions[index],
    note: {
      ciphertextHex: bytesHex(note.ciphertext),
      e2eeGroupId: note.e2eeGroupId,
      epoch: uint64(note.epoch),
      cryptoProfile: note.cryptoProfile,
      envelopeVersion: note.envelopeVersion,
    },
    decisionId: recovery.decisionIds[index],
    approverDeviceId: recovery.approverDeviceIds[index],
  };
}

function recoveryScopeBindingTranscript(envelope) {
  return {
    bindingHashHex: bytesHex(envelope.bindingHash),
    scopeHashHex: envelope.scopeHash,
  };
}

function recoveryEnvelopeTranscript(envelope) {
  return {
    tenantId: envelope.tenantId,
    e2eeGroupId: envelope.groupId,
    epoch: uint64(envelope.epoch),
    cryptoProfile: canonicalProfile(envelope.cryptoProfile),
    recoveryKeyVersion: envelope.recoveryKeyVersion,
    wrappedMaterialHex: bytesHex(envelope.wrappedMaterial),
    envelopeVersion: envelope.envelopeVersion,
    recoveryRecipientPresent: envelope.recoveryRecipientPresent,
    recoveryKeyId: envelope.recoveryKeyId,
    bindingHashHex: bytesHex(envelope.bindingHash),
    scopeHashHex: envelope.scopeHash,
    scopeBindingHashHex: envelope.scopeBindingHash,
    recoveryCaseId: null,
    recoveryRecipientDeviceId: null,
    deliveryPolicyVersion: null,
    deliveryExpiresAt: null,
    deliveryBindingHashHex: null,
    protectedScope: canonicalProtectedScope(envelope.protectedScope),
  };
}

function recoveryDeliveryTranscript(delivery) {
  return {
    tenantId: delivery.tenantId,
    e2eeGroupId: delivery.groupId,
    epoch: uint64(delivery.epoch),
    cryptoProfile: canonicalProfile(delivery.cryptoProfile),
    recoveryKeyVersion: delivery.recoveryKeyVersion,
    wrappedMaterialHex: bytesHex(delivery.wrappedMaterial),
    envelopeVersion: delivery.envelopeVersion,
    recoveryRecipientPresent: delivery.recoveryRecipientPresent,
    recoveryKeyId: delivery.recoveryKeyId,
    bindingHashHex: bytesHex(delivery.bindingHash),
    scopeHashHex: delivery.scopeHash,
    scopeBindingHashHex: delivery.scopeBindingHash,
    recoveryCaseId: delivery.recoveryCaseId,
    recoveryRecipientDeviceId: delivery.recipientDeviceId,
    deliveryPolicyVersion: delivery.deliveryPolicyVersion,
    deliveryExpiresAt: timestamp(delivery.deliveryExpiresAt),
    protectedScope: canonicalProtectedScope(delivery.protectedScope),
  };
}

function recoveryCommitAttestationTranscript(attestation) {
  return {
    tenantId: attestation.tenantId,
    groupId: attestation.groupId,
    epoch: uint64(attestation.epoch),
    scopeHashHex: attestation.scopeHash,
    eventId: attestation.eventId,
    channelSeq: uint64(attestation.channelSeq),
    serverCommittedAt: timestamp(attestation.serverCommittedAt),
    chainHashHex: attestation.chainHash,
    checkpointKeyId: attestation.checkpointKeyId,
    recoveryEnvelopeHashHex: attestation.recoveryEnvelopeHash,
    senderEventBindingHashHex: attestation.senderEventBindingHash,
  };
}

function channelEventSenderBindingTranscript(event, envelope) {
  return {
    eventId: event.eventId,
    tenantId: event.tenantId,
    conversation: {
      channelId: event.channelId ?? null,
      dmId: event.dmId ?? null,
    },
    e2eeGroupId: event.groupId,
    epoch: uint64(event.epoch),
    category: String(eventCategoryWireNumber[event.category] ?? 0),
    senderDeviceId: event.senderDeviceId,
    senderActorId: event.senderActorId,
    idempotencyKey: event.idempotencyKey,
    clientSentAt: timestamp(event.clientSentAt),
    cryptoProfile: event.cryptoProfile,
    envelopeVersion: event.envelopeVersion,
    contentHashHex: event.contentHash,
    recoveryEnvelopeHashHex: envelope
      ? transcriptHash("recovery-envelope", recoveryEnvelopeTranscript(envelope))
      : null,
    redactionTargetEventId: event.redactionTargetEventId,
    attachmentBlobIds: [...event.attachmentBlobIds],
    agentAttribution: event.agentAttribution ? {
      agentActorId: event.agentAttribution.agentActorId,
      taskId: event.agentAttribution.taskId,
      runId: event.agentAttribution.runId,
      capabilityGrantId: event.agentAttribution.capabilityGrantId,
    } : null,
  };
}

function recoveryEvidenceGrantTranscript(grant) {
  return {
    recoveryCaseId: grant.recoveryCaseId,
    tenantId: grant.tenantId,
    approvedScopes: grant.approvedScopes.map(canonicalRecoveryScope),
    recipientDeviceId: grant.recipientDeviceId,
    expiresAt: timestamp(grant.expiresAt),
    policyVersion: grant.policyVersion,
    executionId: grant.executionId,
    caseBindingHashHex: grant.caseBindingHash,
  };
}

const tlMls1Profile = {
  name: "tl-mls-1",
  mlsProtocolVersion: "1.0",
  cipherSuite: "CIPHER_SUITE_MLS_128_DHKEMX25519_AES128GCM_SHA256_ED25519",
  messageEnvelopeVersion: 1,
  historyEnvelopeVersion: 1,
  recoveryEnvelopeVersion: 1,
};

const tlMls2FixtureProfile = {
  name: "tl-mls-2",
  mlsProtocolVersion: "1.0",
  cipherSuite: "CIPHER_SUITE_MLS_128_DHKEMX25519_AES128GCM_SHA256_ED25519",
  messageEnvelopeVersion: 2,
  historyEnvelopeVersion: 1,
  recoveryEnvelopeVersion: 1,
};

function isTlMls1(profile) {
  return canonicalEqual(profile, tlMls1Profile);
}

function isKnownProfile(profile) {
  return isTlMls1(profile) || canonicalEqual(profile, tlMls2FixtureProfile);
}

function read(relativePath) {
  return readFileSync(join(repositoryRoot, relativePath), "utf8").replaceAll("\r\n", "\n");
}

function merge(base, overlay) {
  if (Array.isArray(overlay)) {
    return Array.isArray(base)
      ? overlay.map((value, index) => merge(base[index], value))
      : overlay;
  }
  if (overlay === null || typeof overlay !== "object") return overlay;
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
  if (!isTlMls1(credential.profile)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
  const credentialProjection = credentialTranscript(credential);
  if (!allowedIssuers.has(credential.issuerKind)
    || !credential.issuerAuthorized
    || !credential.approvalSignatureValid
    || credential.issuerKind === "CREDENTIAL_ISSUER_KIND_UNSPECIFIED"
    || credential.issuerKind !== credential.signedIssuerKind
    || credential.approvedBy !== credential.signedApprovedBy
    || credential.signatureInputHash !== transcriptHash("credential", credentialProjection)) {
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
    if (!legacyKeyPackageContext.publishSessionProfiles.some((profile) => canonicalEqual(profile, keyPackage.profile))
      || !canonicalEqual(keyPackage.profile, claim.profile)
      || !isTlMls1(keyPackage.profile)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
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
  if (!keyPackage.profile || typeof keyPackage.profile !== "object") {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  const publicationProjection = publicationTranscript(keyPackage);
  const expectedPublicationBinding = transcriptHash("publication", publicationProjection);
  const expectedPublicationSignature = transcriptHash("publication", {
    deviceId: keyPackage.deviceId,
    publicationBindingHashHex: expectedPublicationBinding,
  });
  if (keyPackage.publicationBindingHash !== expectedPublicationBinding
    || keyPackage.publicationSignature !== expectedPublicationSignature) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (keyPackage.tenantId !== claim.tenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
  if (keyPackage.deviceId !== claim.deviceId) return reject("ERROR_CODE_DEVICE_NOT_ENROLLED");
  if (keyPackage.credentialVersion !== claim.credentialVersion) return reject("ERROR_CODE_DEVICE_REVOKED");
  if (!canonicalEqual(keyPackage.profile, claim.profile) || !isTlMls1(keyPackage.profile)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
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
  if (!isTlMls1(membership.groupProfile)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
  if (membership.tenantId !== membership.groupTenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
  if (membership.callerDeviceId !== membership.committerDeviceId) return reject("ERROR_CODE_PERMISSION_DENIED");
  if (now > membership.expiresAt) return reject("ERROR_CODE_GRANT_EXPIRED");
  if (!membership.bindingHashValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  const actualBindingHash = transcriptHash("membership", membershipTranscript(membership, changeKind));
  const boundBindingHash = transcriptHash("membership", membershipTranscript(
    membership.authorizationBinding,
    membership.authorizationBinding.kind,
  ));
  const authorizationBindingValid = membership.authorizationBindingHash === actualBindingHash
    && membership.authorizationBindingHash === boundBindingHash;
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
    if (!authorizationBindingValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_REMOVE_DEVICE") {
    if (membership.removeDeviceIds.length === 0 || !noAdds || !noUpdates || !noRecoveryRotation || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (!authorizationBindingValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_REVOKE_DEVICE") {
    if ((noRemoves && noUpdates) || !noAdds || !noRecoveryRotation || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (!authorizationBindingValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
    return { status: "accepted", successorEpoch: membership.successorEpoch, changeSeq: membership.changeSeq, invalidatesUnusedKeyPackages: true };
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_UPDATE_DEVICE_KEY" || changeKind === "MEMBERSHIP_CHANGE_KIND_SELF_UPDATE") {
    if (membership.updateDeviceIds.length === 0 || !noAdds || !noRemoves || !noRecoveryRotation || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (!authorizationBindingValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_RECOVERY_KEY_ROTATION") {
    if (!noAdds || !noRemoves || !noUpdates || !noReinitialization) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (membership.successorRecoveryKeyVersion <= membership.currentRecoveryKeyVersion) return reject("ERROR_CODE_REPLAY_DETECTED");
    if (!authorizationBindingValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  if (changeKind === "MEMBERSHIP_CHANGE_KIND_REINITIALIZE") {
    if (!authorizationBindingValid && (!membership.successorProfile
      || !membership.successorE2eeGroupId
      || !membership.successorGroup.groupId)) {
      return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
    }
    if (!noAdds || !noRemoves || !noUpdates || !noRecoveryRotation) return reject("ERROR_CODE_PERMISSION_DENIED");
    if (membership.successorGroup.tenantId !== membership.groupTenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
    if (membership.successorGroup.parent !== membership.groupParent
      || membership.successorGroup.groupId !== membership.successorE2eeGroupId
      || membership.currentSuccessorE2eeGroupId !== membership.successorE2eeGroupId
      || membership.successorGroup.predecessorGroupId !== membership.groupId) return reject("ERROR_CODE_GROUP_MISMATCH");
    if (!membership.successorProfile
      || !isKnownProfile(membership.successorProfile)
      || canonicalEqual(membership.successorProfile, membership.groupProfile)
      || !canonicalEqual(membership.successorGroup.profile, membership.successorProfile)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
    if (membership.successorGroup.generation !== membership.groupGeneration + 1) {
      return reject(membership.successorGroup.generation > membership.groupGeneration + 1 ? "ERROR_CODE_EPOCH_AHEAD" : "ERROR_CODE_EPOCH_STALE");
    }
    if (!authorizationBindingValid) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
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
  if (!isTlMls1(history.profile)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
  if (!history.sourceEligible) return { status: "unavailable", grants: 0, automaticPrejoinAccess: false };
  if (history.tenantId !== history.groupTenantId || history.tenantId !== history.targetTenantId) return reject("ERROR_CODE_TENANT_MISMATCH");
  const historyProjection = historyTranscript(history);
  if (!history.targetAuthorized || !history.signatureValid) return reject("ERROR_CODE_HISTORY_SHARING_DENIED");
  if (history.firstEpoch !== history.requestedFirstEpoch
    || history.lastEpoch !== history.requestedLastEpoch
    || history.policyVersion !== history.requestPolicyVersion) return reject("ERROR_CODE_HISTORY_SHARING_DENIED");
  if (history.firstEpoch < history.retentionFirstEpoch || history.lastEpoch > history.retentionLastEpoch) {
    return reject("ERROR_CODE_RETENTION_EXPIRED");
  }
  if (history.firstEpoch > history.lastEpoch) return reject("ERROR_CODE_EPOCH_UNAVAILABLE");
  if (now > Math.min(history.requestExpiresAt, history.grantExpiresAt, history.retentionExpiresAt)) return reject("ERROR_CODE_GRANT_EXPIRED");
  if (history.signatureInputHash !== transcriptHash("history", historyProjection)) {
    return reject("ERROR_CODE_HISTORY_SHARING_DENIED");
  }
  return { status: "accepted", firstEpoch: history.firstEpoch, lastEpoch: history.lastEpoch };
}

function evaluateRecovery(testCase) {
  const { recovery, now } = testCase;
  if (!recovery.explicitTargetCapabilityObserved) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
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
    || !canonicalEqual(canonicalScopeIds, sortedScopeIds)) {
    return reject("ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT");
  }
  const sharedScope = recovery.scopes[0];
  if (recovery.scopes.some((scope) => scope.firstEpoch !== sharedScope.firstEpoch
    || scope.lastEpoch !== sharedScope.lastEpoch
    || scope.startTime !== sharedScope.startTime
    || scope.endTime !== sharedScope.endTime)) {
    return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  }
  for (const scope of recovery.scopes) {
    if (scope.targets.length === 0) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
    const canonicalTargets = scope.targets.map(canonicalProtectedScope);
    const sortedTargets = [...scope.targets].sort(compareProtectedScopes).map(canonicalProtectedScope);
    if (!canonicalEqual(canonicalTargets, sortedTargets)
      || new Set(canonicalTargets.map(canonicalize)).size !== canonicalTargets.length) {
      return reject("ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT");
    }
    for (const target of scope.targets) {
      const hasTargetObject = Boolean(target.object);
      const hasTargetHistory = Boolean(target.historyEpochRange);
      if (hasTargetObject === hasTargetHistory) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
      if (hasTargetObject && (!target.object.resourceId
        || !["RECOVERY_OBJECT_KIND_EVENT", "RECOVERY_OBJECT_KIND_BLOB", "RECOVERY_OBJECT_KIND_ARTIFACT"].includes(target.object.kind))) {
        return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
      }
      if (hasTargetHistory && (target.historyEpochRange.groupId !== scope.groupId
        || target.historyEpochRange.firstEpoch < scope.firstEpoch
        || target.historyEpochRange.lastEpoch > scope.lastEpoch
        || target.historyEpochRange.firstEpoch > target.historyEpochRange.lastEpoch)) {
        return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
      }
    }
  }
  const legacyProjectionPoisoned = recovery.legacyGroupIds.length === 0
    && recovery.legacyFirstEpoch === 0
    && recovery.legacyLastEpoch === 0
    && recovery.legacyStartTime === null
    && recovery.legacyEndTime === null;
  if (!legacyProjectionPoisoned) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  const caseApprovalProjection = recoveryCaseTranscript(recovery);
  const expectedCaseBindingHash = transcriptHash("recovery-case", caseApprovalProjection);
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
      const decisionProjection = recoveryDecisionTranscript(recovery, index, expectedCaseBindingHash);
      return recovery.decisionSignatures[index] !== transcriptHash("recovery-decision", decisionProjection);
    })) {
    return reject("ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT");
  }
  if (recovery.approverDeviceIds.some((deviceId, index) => deviceId !== recovery.approvalSessionDeviceIds[index])) {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  const executionScope = recovery.scopes.find((scope) => scope.groupId === recovery.envelope.groupId);
  if (!executionScope) return reject("ERROR_CODE_GROUP_MISMATCH");
  if (!isTlMls1(recovery.envelope.cryptoProfile)) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED");
  const protectedScope = recovery.envelope.protectedScope;
  if (!protectedScope) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  const hasObject = Boolean(protectedScope.object);
  const hasHistory = Boolean(protectedScope.historyEpochRange);
  if (hasObject === hasHistory) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  if (hasObject && (!protectedScope.object.resourceId
    || !["RECOVERY_OBJECT_KIND_EVENT", "RECOVERY_OBJECT_KIND_BLOB", "RECOVERY_OBJECT_KIND_ARTIFACT"].includes(protectedScope.object.kind))) {
    return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  }
  if (hasHistory && (protectedScope.historyEpochRange.groupId !== recovery.envelope.groupId
    || protectedScope.historyEpochRange.firstEpoch > protectedScope.historyEpochRange.lastEpoch
    || recovery.envelope.epoch < protectedScope.historyEpochRange.firstEpoch
    || recovery.envelope.epoch > protectedScope.historyEpochRange.lastEpoch)) {
    return reject("ERROR_CODE_EPOCH_UNAVAILABLE");
  }
  const protectedTargetApproved = executionScope.targets.some((target) => {
    if (hasObject) return Boolean(target.object) && canonicalEqual(target.object, protectedScope.object);
    if (!target.historyEpochRange) return false;
    return target.historyEpochRange.groupId === protectedScope.historyEpochRange.groupId
      && protectedScope.historyEpochRange.firstEpoch >= target.historyEpochRange.firstEpoch
      && protectedScope.historyEpochRange.lastEpoch <= target.historyEpochRange.lastEpoch;
  });
  if (hasHistory && !protectedTargetApproved) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  if (recovery.envelope.epoch < executionScope.firstEpoch || recovery.envelope.epoch > executionScope.lastEpoch) {
    return reject("ERROR_CODE_EPOCH_UNAVAILABLE");
  }
  if (!recovery.envelope.scopeHash || !recovery.envelope.scopeBindingHash) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  const canonicalScope = canonicalProtectedScope(protectedScope);
  const expectedScopeHash = transcriptHash("recovery-protected-scope", canonicalScope);
  const expectedScopeBindingHash = transcriptHash("recovery-scope-binding", recoveryScopeBindingTranscript({
    ...recovery.envelope,
    scopeHash: expectedScopeHash,
  }));
  if (recovery.envelope.scopeHash !== expectedScopeHash
    || recovery.envelope.scopeBindingHash !== expectedScopeBindingHash) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (!recovery.envelope.recoveryRecipientPresent) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  if (!recovery.evidenceFetchAuthorized || recovery.evidenceCallerIdentity !== "threadline-recovery-control") {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  const evidenceGrantInput = recoveryEvidenceGrantTranscript(recovery.evidenceGrant);
  if (now > recovery.evidenceGrant.expiresAt) return reject("ERROR_CODE_GRANT_EXPIRED");
  if (recovery.evidenceGrant.expiresAt > recovery.expiresAt) return reject("ERROR_CODE_PERMISSION_DENIED");
  if (!recovery.evidenceGrantSignatureVerified
    || recovery.evidenceGrantSignatureInputHash !== transcriptHash("recovery-evidence-grant", evidenceGrantInput)) {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (recovery.evidenceGrant.recoveryCaseId !== recovery.caseId
    || recovery.evidenceGrant.tenantId !== recovery.tenantId
    || recovery.evidenceGrant.recipientDeviceId !== recovery.recipientDeviceId
    || recovery.evidenceGrant.policyVersion !== recovery.policyVersion
    || recovery.evidenceGrant.executionId !== recovery.executionId
    || recovery.evidenceGrant.caseBindingHash !== expectedCaseBindingHash) {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (!canonicalEqual(recovery.evidenceGrant.approvedScopes, recovery.scopes)) {
    return reject("ERROR_CODE_RECOVERY_APPROVAL_INSUFFICIENT");
  }
  if (!recovery.evidenceFetched || recovery.evidenceAttestationCount !== 1
    || recovery.attestations.length !== 1) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  const attestation = recovery.attestations[0];
  if (attestation.tenantId !== recovery.tenantId
    || attestation.groupId !== recovery.envelope.groupId
    || attestation.epoch !== recovery.envelope.epoch
    || attestation.scopeHash !== recovery.envelope.scopeHash) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (attestation.serverCommittedAt < executionScope.startTime
    || attestation.serverCommittedAt >= executionScope.endTime) return reject("ERROR_CODE_EPOCH_UNAVAILABLE");
  const expectedEnvelopeHash = transcriptHash("recovery-envelope", recoveryEnvelopeTranscript(recovery.envelope));
  const expectedSenderEventBindingHash = transcriptHash(
    "channel-event-sender-binding",
    channelEventSenderBindingTranscript(recovery.senderEvent, recovery.envelope),
  );
  if (attestation.eventId !== recovery.expectedAttestationEventId
    || attestation.recoveryEnvelopeHash !== expectedEnvelopeHash
    || recovery.senderEvent.eventId !== attestation.eventId
    || !recovery.senderEvent.senderSignatureVerified
    || attestation.senderEventBindingHash !== expectedSenderEventBindingHash
    || !attestation.senderEventBindingHash
    || attestation.chainHash !== recovery.expectedAttestationChainHash
    || !recovery.authorizedCheckpointKeyIds.includes(attestation.checkpointKeyId)) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  const attestationProjection = recoveryCommitAttestationTranscript(attestation);
  const attestationInputHash = transcriptHash("recovery-commit-attestation", attestationProjection);
  if (attestation.checkpointSignatureInputHash !== attestationInputHash
    || !attestation.checkpointSignatureVerified) return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  if (now > recovery.delivery.deliveryExpiresAt || recovery.delivery.deliveryExpiresAt > recovery.expiresAt) return reject("ERROR_CODE_RECOVERY_CASE_EXPIRED");
  const deliveryBindingProjection = recoveryDeliveryTranscript(recovery.delivery);
  if (recovery.delivery.deliveryBindingHash !== transcriptHash("recovery-delivery", deliveryBindingProjection)) {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  if (hasObject && !protectedTargetApproved) return reject("ERROR_CODE_RECOVERY_UNAVAILABLE");
  const projectionMatches = recovery.delivery.recoveryCaseId === recovery.caseId
    && recovery.delivery.tenantId === recovery.envelope.tenantId
    && recovery.delivery.groupId === recovery.envelope.groupId
    && recovery.delivery.epoch === recovery.envelope.epoch
    && recovery.delivery.recipientDeviceId === recovery.recipientDeviceId
    && recovery.delivery.deliveryPolicyVersion === recovery.policyVersion
    && canonicalEqual(recovery.delivery.cryptoProfile, recovery.envelope.cryptoProfile)
    && recovery.delivery.recoveryKeyVersion === recovery.envelope.recoveryKeyVersion
    && recovery.delivery.recoveryKeyId === recovery.envelope.recoveryKeyId
    && recovery.delivery.bindingHash === recovery.envelope.bindingHash
    && recovery.delivery.scopeHash === recovery.envelope.scopeHash
    && recovery.delivery.scopeBindingHash === recovery.envelope.scopeBindingHash
    && canonicalEqual(recovery.delivery.protectedScope, recovery.envelope.protectedScope);
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
  assert(!legacy.scopeHashPresent && !legacy.scopeBindingHashPresent
    && !legacy.deliveryMetadataPresent && !legacy.protectedScopePresent,
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
  return reject("ERROR_CODE_RECOVERY_UNAVAILABLE", {
    auditScopes: groupIds.map((groupId) => ({
      groupId,
      firstEpoch: legacy.firstEpoch,
      lastEpoch: legacy.lastEpoch,
      startTime: legacy.startTime,
      endTime: legacy.endTime,
    })),
    executable: false,
    targetsInferred: false,
  });
}

function evaluateRecoveryCapabilities(testCase) {
  const capability = testCase.recoveryCapability;
  if (capability.rpcStatus === "UNIMPLEMENTED"
    || !capability.explicitProtectedTargets
    || capability.contractVersion !== 1) {
    return reject("ERROR_CODE_RECOVERY_UNAVAILABLE", { readOnly: true });
  }
  return { status: "accepted", contractVersion: 1, explicitProtectedTargets: true };
}

function evaluateLegacyRecoveryTargets(testCase) {
  const legacy = testCase.legacyRecoveryTargets;
  assert(!legacy.targetsPresent, `${testCase.id}: legacy target fixture must omit protected targets`);
  return reject("ERROR_CODE_RECOVERY_UNAVAILABLE", {
    operation: legacy.operation,
    decoded: true,
    auditReadable: true,
    executable: false,
    targetsInferred: false,
  });
}

// Models the recovery Group/window admission logic at base 6ee924d after an
// N-1 binary ignores unknown current scopes/targets. The intentionally empty
// legacy projection poisons every mutating path, including rollback reads of a
// persisted current Case.
function evaluateNMinusOneRecovery(testCase) {
  const legacy = testCase.nMinusOneRecovery;
  assert(legacy.unknownCurrentScopesIgnored, `${testCase.id}: N-1 simulation must ignore unknown scopes`);
  const poisoned = legacy.e2eeGroupIds.length === 0
    && legacy.firstEpoch === 0
    && legacy.lastEpoch === 0
    && !legacy.timeRangePresent;
  assert(poisoned, `${testCase.id}: current writer must poison the legacy recovery projection`);
  return reject("ERROR_CODE_RECOVERY_UNAVAILABLE", {
    operation: legacy.operation,
    simulatedBase: "6ee924d",
    decoded: true,
    auditReadable: true,
    mutationPerformed: false,
    broadWindowInferred: false,
  });
}

function evaluateNMinusOneMatrix(testCase) {
  const legacy = testCase.nMinusOne;
  if (legacy.surface === "device-credential") {
    if (!legacy.missingFields11And12 || !legacy.legacyFields1To8SignatureValid
      || legacy.approvalRoots.length !== 1 || legacy.approvalRoots[0] !== legacy.approvedBy) {
      return reject("ERROR_CODE_PERMISSION_DENIED", { normalized: false });
    }
    if (legacy.attemptsCurrentSigningAction) {
      return reject("ERROR_CODE_PERMISSION_DENIED", { legacy: true, requiresReissue: true });
    }
    return {
      status: "readable",
      legacy: true,
      formatVersion: 0,
      requiresReissue: true,
      verificationIssuerKind: legacy.resolvedIssuerKind,
    };
  }
  if (legacy.surface === "e2ee-group") {
    if (!legacy.missingFields10And11) return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED", { readOnly: true });
    if (legacy.isSuccessor || legacy.hasReinitializationLink) {
      return reject("ERROR_CODE_CRYPTO_PROFILE_UNSUPPORTED", { readOnly: true });
    }
    return { status: "accepted", normalizedGeneration: 1, predecessorGroupId: "" };
  }
  if (legacy.surface === "membership-authorization") {
    assert(legacy.missingFields11To18, `${testCase.id}: fixture must omit current membership bindings`);
    return reject("ERROR_CODE_PERMISSION_DENIED", { auditReadable: true, executable: false, action: "request-current-authorization" });
  }
  if (legacy.surface === "mls-wire") {
    const outerBound = legacy.missingFields7To13 && legacy.authenticatedChannelEvent
      && legacy.tenantId && legacy.groupId && legacy.senderDeviceId && legacy.eventId;
    if (!outerBound) return reject("ERROR_CODE_PERMISSION_DENIED", { normalized: false, auditOnly: true });
    if (legacy.messageType === "MLS_WIRE_MESSAGE_TYPE_COMMIT") {
      if (legacy.wireFormat !== "MLS_WIRE_FORMAT_PRIVATE_MESSAGE"
        || !legacy.authorizationId || !legacy.exactPendingAuthorization
        || legacy.successorEpoch !== legacy.epoch + 1) return reject("ERROR_CODE_PERMISSION_DENIED", { normalized: false, auditOnly: true });
    } else if (legacy.messageType === "MLS_WIRE_MESSAGE_TYPE_PROPOSAL") {
      if (legacy.wireFormat !== "MLS_WIRE_FORMAT_PRIVATE_MESSAGE"
        || legacy.authorizationId || legacy.successorEpoch !== 0) return reject("ERROR_CODE_PERMISSION_DENIED", { normalized: false, auditOnly: true });
    } else if (legacy.messageType === "MLS_WIRE_MESSAGE_TYPE_WELCOME") {
      if (legacy.wireFormat !== "MLS_WIRE_FORMAT_INDEPENDENT"
        || legacy.targetDeviceIds.length === 0 || legacy.acceptedCommitMatches !== 1) {
        return reject("ERROR_CODE_PERMISSION_DENIED", { normalized: false, auditOnly: true });
      }
    } else if (legacy.messageType === "MLS_WIRE_MESSAGE_TYPE_GROUP_INFO") {
      if (legacy.wireFormat !== "MLS_WIRE_FORMAT_INDEPENDENT"
        || legacy.relatedCommitMatches > 1) return reject("ERROR_CODE_PERMISSION_DENIED", { normalized: false, auditOnly: true });
    } else return reject("ERROR_CODE_ENVELOPE_VERSION_UNSUPPORTED", { normalized: false, auditOnly: true });
    return { status: "accepted", normalized: true, replayIdentity: legacy.eventId, messageType: legacy.messageType };
  }
  if (legacy.surface === "history-grant") {
    assert(legacy.missingFields12To15, `${testCase.id}: fixture must omit current History bindings`);
    return reject("ERROR_CODE_HISTORY_SHARING_DENIED", { auditReadable: true, unwrapAllowed: false, action: "request-current-grant" });
  }
  throw new Error(`${testCase.id}: unknown N-1 surface ${legacy.surface}`);
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
  for (const fieldNumber of [13, 14, 15, 16, 17, 18]) {
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
  if (testCase.kind === "recovery-capability") return evaluateRecoveryCapabilities(testCase);
  if (testCase.kind === "legacy-recovery-targets") return evaluateLegacyRecoveryTargets(testCase);
  if (testCase.kind === "n-minus-one-recovery") return evaluateNMinusOneRecovery(testCase);
  if (testCase.kind === "n-minus-one-matrix") return evaluateNMinusOneMatrix(testCase);
  if (testCase.kind === "compatibility") return evaluateCompatibility(testCase);
  throw new Error(`${testCase.id}: unknown case kind ${testCase.kind}`);
}

const rawCases = new Map(scenarios.cases.map((testCase) => [testCase.id, testCase]));
assert(rawCases.size === scenarios.cases.length, "scenario IDs must be unique");

function materializeCase(id) {
  const raw = rawCases.get(id);
  assert(raw, `missing representative scenario ${id}`);
  const testCase = merge(scenarios.defaults, raw);
  testCase.id = raw.id;
  testCase.kind = raw.kind;
  testCase.expected = raw.expected;
  return testCase;
}

const transcriptBuilders = {
  credential: (testCase) => credentialTranscript(testCase.credential),
  publication: (testCase) => publicationTranscript(testCase.keyPackage),
  membership: (testCase) => membershipTranscript(testCase.membership, testCase.changeKind),
  history: (testCase) => historyTranscript(testCase.history),
  "recovery-case": (testCase) => recoveryCaseTranscript(testCase.recovery),
  "recovery-decision": (testCase) => {
    const caseHash = transcriptHash("recovery-case", recoveryCaseTranscript(testCase.recovery));
    return recoveryDecisionTranscript(testCase.recovery, 0, caseHash);
  },
  "recovery-protected-scope": (testCase) => canonicalProtectedScope(testCase.recovery.envelope.protectedScope),
  "recovery-scope-binding": (testCase) => recoveryScopeBindingTranscript(testCase.recovery.envelope),
  "recovery-envelope": (testCase) => recoveryEnvelopeTranscript(testCase.recovery.envelope),
  "channel-event-sender-binding": (testCase) => channelEventSenderBindingTranscript(
    testCase.recovery.senderEvent,
    testCase.recovery.envelope,
  ),
  "recovery-evidence-grant": (testCase) => recoveryEvidenceGrantTranscript(testCase.recovery.evidenceGrant),
  "recovery-delivery": (testCase) => recoveryDeliveryTranscript(testCase.recovery.delivery),
  "recovery-commit-attestation": (testCase) => recoveryCommitAttestationTranscript(testCase.recovery.attestations[0]),
};

function liveTranscriptVectors() {
  return Object.entries(manifest.transcripts.representativeCases).map(([domain, scenarioId]) => {
    const input = transcriptBuilders[domain](materializeCase(scenarioId));
    const bytes = canonicalTranscript(domain, input);
    return { domain, scenarioId, input, canonicalHex: bytes.toString("hex"), sha256: sha256(bytes) };
  });
}

function liveMembershipKindVectors() {
  return Object.entries(manifest.transcripts.membershipKindCases).map(([kind, pin]) => {
    const testCase = materializeCase(pin.scenarioId);
    assert(testCase.changeKind === kind, `${kind}: representative scenario kind mismatch`);
    const input = membershipTranscript(testCase.membership, testCase.changeKind);
    const bytes = canonicalTranscript("membership", input);
    return { kind, scenarioId: pin.scenarioId, input, canonicalHex: bytes.toString("hex"), sha256: sha256(bytes) };
  });
}

if (process.argv.includes("--print-transcripts")) {
  console.log(`${JSON.stringify({
    schemaVersion: 1,
    format: "RFC8785-JCS",
    domainPrefix: transcriptPrefix,
    vectors: liveTranscriptVectors(),
    membershipKindVectors: liveMembershipKindVectors(),
  }, null, 2)}\n`);
  process.exit(0);
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
assert(sha256(transcriptBytesFixture) === manifest.transcripts.sha256, "canonical transcript source SHA-256 mismatch");
assert(manifest.transcripts.format === "RFC8785-JCS" && manifest.transcripts.domainPrefix === transcriptPrefix, "canonical transcript format metadata changed");
assert(manifest.transcripts.generator === "node proto/tools/verify-crypto-contracts.mjs --print-transcripts", "canonical transcript generator must remain live-builder-backed");
for (const requiredFile of manifest.requiredFiles) assert(statSync(join(fixtureRoot, requiredFile)).isFile(), `missing fixture file ${requiredFile}`);
const transcriptDomains = ["credential", "publication", "membership", "history", "recovery-case", "recovery-decision", "recovery-protected-scope", "recovery-scope-binding", "recovery-envelope", "channel-event-sender-binding", "recovery-evidence-grant", "recovery-delivery", "recovery-commit-attestation"];
assert(canonicalEqual(transcriptVectors.vectors.map((vector) => vector.domain), transcriptDomains), "canonical transcript domain set/order changed");
const liveVectors = liveTranscriptVectors();
for (const [index, vector] of transcriptVectors.vectors.entries()) {
  assert(vector.scenarioId === liveVectors[index].scenarioId, `${vector.domain}: representative scenario changed`);
  assert(canonicalEqual(vector.input, liveVectors[index].input), `${vector.domain}: vector input differs from live production projection builder`);
  const bytes = canonicalTranscript(vector.domain, vector.input);
  assert(bytes.toString("hex") === vector.canonicalHex, `${vector.domain}: canonical transcript bytes changed`);
  assert(sha256(bytes) === vector.sha256, `${vector.domain}: canonical transcript SHA-256 changed`);
}
const liveKindVectors = liveMembershipKindVectors();
assert(canonicalEqual(transcriptVectors.membershipKindVectors, liveKindVectors), "membership kind vectors differ from live builders");
for (const vector of liveKindVectors) {
  assert(vector.sha256 === manifest.transcripts.membershipKindCases[vector.kind].sha256, `${vector.kind}: pinned membership hash changed`);
}
assert(scenarios.schemaVersion === manifest.schemaVersion && scenarios.classification === manifest.classification, "scenario metadata differs from manifest");
for (const contract of manifest.contracts) assert(statSync(join(repositoryRoot, "proto", contract)).isFile(), `missing contract ${contract}`);

const crypto = read("proto/threadline/crypto/v1/crypto.proto");
const recovery = read("proto/threadline/crypto/v1/recovery.proto");
const services = read("proto/threadline/crypto/v1/key_service.proto");
const errors = read("proto/threadline/type/v1/error.proto");

for (const [name, wireNumber] of Object.entries(membershipKindWireNumber)) {
  assert(new RegExp(`${name}\\s*=\\s*${wireNumber};`, "u").test(crypto), `${name}: verifier wire number differs from Proto`);
}

for (const assertion of [
  [crypto, /uint32\s+credential_format_version\s*=\s*11;/u, "Device Credential format version"],
  [crypto, /CredentialIssuerKind\s+issuer_kind\s*=\s*12;/u, "Device Credential issuer"],
  [crypto, /`tl-mls-1` is exactly the full tuple:[\s\S]*MLS protocol "1\.0"[\s\S]*envelope versions all equal to 1[\s\S]*same-name mismatch[\s\S]*fail closed/u, "exact tl-mls-1 profile tuple"],
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
  [recovery, /repeated\s+RecoveryProtectedScope\s+protected_targets\s*=\s*5;/u, "explicit typed Recovery targets"],
  [recovery, /bytes\s+case_binding_hash\s*=\s*8;/u, "Recovery Approval Case binding"],
  [recovery, /bytes\s+decision_signature\s*=\s*9;/u, "Recovery Approval decision signature"],
  [recovery, /string\s+approver_device_id\s*=\s*10;/u, "Recovery Approval signing Device"],
  [recovery, /case_binding_hash[\s\S]*DecideRecoveryCaseRequest fields 1-4 and 7[\s\S]*Service-generated approval_id[\s\S]*excluded/u, "client-generatable Recovery decision signature projection"],
  [recovery, /requester ActorRef identity[\s\S]*canonical `scopes`[\s\S]*Poisoned legacy fields[\s\S]*empty\/default values/u, "authoritative Recovery Case approval projection"],
  [recovery, /complete encrypted note \(ciphertext, e2ee_group_id, epoch,[\s\S]*crypto_profile and envelope_version\)/u, "complete encrypted approval-note signature binding"],
  [recovery, /ascending bytewise e2ee_group_id order[\s\S]*same shared inclusive Epoch bounds and half-open TimeRange[\s\S]*poison[\s\S]*empty\/default values/u, "canonical targetful Recovery scopes with poisoned legacy projection"],
  [recovery, /bytes\s+scope_hash\s*=\s*11;/u, "Recovery Envelope scope hash"],
  [recovery, /bytes\s+scope_binding_hash\s*=\s*12;/u, "Recovery Envelope scope-aware binding"],
  [recovery, /string\s+recovery_case_id\s*=\s*13;/u, "RecoveryEnvelope delivery Case binding"],
  [recovery, /string\s+recovery_recipient_device_id\s*=\s*14;/u, "RecoveryEnvelope delivery recipient"],
  [recovery, /string\s+delivery_policy_version\s*=\s*15;/u, "RecoveryEnvelope delivery policy"],
  [recovery, /google\.protobuf\.Timestamp\s+delivery_expires_at\s*=\s*16;/u, "RecoveryEnvelope delivery expiry"],
  [recovery, /bytes\s+delivery_binding_hash\s*=\s*17;/u, "RecoveryEnvelope current delivery binding"],
  [recovery, /message\s+RecoveryObjectRef\s*\{[\s\S]*RecoveryObjectKind\s+kind\s*=\s*1;[\s\S]*resource_id\s*=\s*2;/u, "typed Recovery object reference"],
  [recovery, /message\s+RecoveryProtectedScope\s*\{[\s\S]*oneof\s+target[\s\S]*RecoveryObjectRef\s+object\s*=\s*1;/u, "sender-authored Recovery protected scope"],
  [recovery, /message\s+RecoveryScopeCommitAttestation\s*\{[\s\S]*server_committed_at\s*=\s*7;[\s\S]*checkpoint_signature\s*=\s*10;[\s\S]*recovery_envelope_hash\s*=\s*11;[\s\S]*sender_event_binding_hash\s*=\s*12;/u, "post-commit Recovery scope attestation"],
  [recovery, /message\s+RecoveryEvidenceGrant\s*\{[\s\S]*approved_scopes\s*=\s*3;[\s\S]*case_binding_hash\s*=\s*8;[\s\S]*recovery_control_signature\s*=\s*9;/u, "signed minimal Recovery evidence grant"],
  [recovery, /RecoveryProtectedScope\s+protected_scope\s*=\s*18;/u, "RecoveryEnvelope protected scope field"],
  [recovery, /fields 1-16[\s\S]*and 18[\s\S]*Field 17 is the digest itself and is excluded/u, "complete RecoveryEnvelope delivery projection"],
  [services, /MessageService\.SendEvent/u, "single durable MLS sequencing path"],
  [services, /string\s+claim_id\s*=\s*4;/u, "atomic claim replay identity"],
  [services, /message\s+DecideRecoveryCaseRequest\s*\{[\s\S]*bytes\s+case_binding_hash\s*=\s*5;[\s\S]*bytes\s+decision_signature\s*=\s*6;[\s\S]*string\s+approver_device_id\s*=\s*7;/u, "approver-submitted Recovery Case binding and Device signature"],
  [services, /message\s+ExecuteRecoveryCaseRequest\s*\{[\s\S]*Empty is invalid[\s\S]*string\s+execution_id\s*=\s*2;/u, "non-empty Recovery execution idempotency key"],
  [services, /rpc\s+GetRecoveryCapabilities[\s\S]*explicit_protected_targets\s*=\s*1;[\s\S]*contract_version\s*=\s*2;/u, "server-first Recovery target capability"],
  [services, /service\s+RecoveryEvidenceService\s*\{[\s\S]*GetRecoveryEvidence[\s\S]*RecoveryEvidenceGrant\s+grant\s*=\s*1;[\s\S]*repeated\s+RecoveryEvidenceItem\s+items\s*=\s*1;[\s\S]*RecoveryEnvelope\s+envelope\s*=\s*1;[\s\S]*RecoveryScopeCommitAttestation\s+attestation\s*=\s*2;/u, "Core Recovery evidence fetch seam"],
  [services, /Poisoned N-1 projection[\s\S]*leave fields 2-5[\s\S]*old server sees no Group and rejects/u, "v1 poisoned Recovery request projection"],
  [services, /repeated\s+RecoveryEnvelope\s+delivered_envelopes\s*=\s*2;/u, "single N-1/current recovery delivery surface"],
  [crypto, /Required and non-empty for WELCOME[\s\S]*only for non-WELCOME message types/u, "WELCOME target requirement"],
  [services, /HistoryService and the internal[\s\S]*RecoveryEvidenceService are served by[\s\S]*threadline-core[\s\S]*RecoveryService is the only recovery-control service[\s\S]*threadline-recovery-control/u, "History and Recovery service ownership boundary"],
]) assert(assertion[1].test(assertion[0]), `contract is missing ${assertion[2]}`);
assert(!/message\s+ExecuteRecoveryCaseRequest\s*\{[^}]*scope_commit_attestations/u.test(services), "Execute caller must not submit Recovery attestations");
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
