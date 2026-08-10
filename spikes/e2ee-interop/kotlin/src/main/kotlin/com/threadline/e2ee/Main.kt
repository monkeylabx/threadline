package com.threadline.e2ee

import java.nio.file.Files
import java.nio.file.Path
import java.security.MessageDigest

private class GoldenVector private constructor(
    private val values: Map<String, String>,
) {
    companion object {
        fun load(path: Path): GoldenVector {
            val parsed = linkedMapOf<String, String>()
            Files.readAllLines(path).forEachIndexed { index, rawLine ->
                val line = rawLine.trim()
                if (line.isEmpty() || line.startsWith("#")) return@forEachIndexed
                val separator = line.indexOf('=')
                require(separator > 0 && separator < line.lastIndex) {
                    "invalid vector line: ${index + 1}"
                }
                val key = line.substring(0, separator)
                val value = line.substring(separator + 1)
                require(parsed.put(key, value) == null) { "duplicate vector key: $key" }
            }
            return GoldenVector(parsed)
        }
    }

    private fun value(key: String): String =
        requireNotNull(values[key]) { "missing vector key: $key" }

    private fun expect(key: String, expected: String) {
        val actual = value(key)
        require(actual == expected) {
            "vector mismatch for $key: expected $expected, got $actual"
        }
    }

    fun validate() {
        expect("format_version", "1")
        expect("profile", "tl-mls-1")
        expect("protocol", "MLS-1.0-RFC9420")
        expect("ciphersuite", "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519")
        expect("mls_library", "openmls")
        expect("mls_library_version", "0.8.1")
        expect("vector_class", "semantic-contract")
        expect("recovery.wrapper", "optional")
        expect("recovery.private_key_location", "external-kms-hsm-only")
        expect("output.classification", "public-metadata-and-digests-only")

        mapOf(
            "key_package.expected" to "accept",
            "offline_new_device.expected" to "join-current-epoch",
            "out_of_order_commit.expected" to "queue-until-predecessor",
            "device_revocation.expected" to "removed-device-cannot-read-future-epoch",
            "history.authorized" to "accept",
            "history.unauthorized" to "TL_E2EE_HISTORY_UNAUTHORIZED",
            "history.cross_tenant" to "TL_E2EE_TENANT_MISMATCH",
            "recovery.success" to "accept",
            "recovery.no_recipient" to "TL_E2EE_RECOVERY_UNAVAILABLE",
            "recovery.refused" to "TL_E2EE_RECOVERY_REFUSED",
            "recovery.corrupt" to "TL_E2EE_CORRUPT",
            "recovery.old_epoch" to "TL_E2EE_OLD_EPOCH",
            "recovery.cross_group" to "TL_E2EE_GROUP_MISMATCH",
            "recovery.cross_tenant" to "TL_E2EE_TENANT_MISMATCH",
        ).forEach { (key, expected) -> expect(key, expected) }

        listOf(
            "error.replay",
            "error.corrupt",
            "error.old_epoch",
            "error.future_epoch",
            "error.unknown_version",
        ).forEach { key ->
            val code = value(key)
            require(code.startsWith("TL_E2EE_")) {
                "vector mismatch for $key: expected stable TL_E2EE_* error, got $code"
            }
        }

        val canonical = listOf(
            "tl-recovery-envelope-v${value("recovery.version")}",
            value("recovery.tenant_id"),
            value("recovery.group_id"),
            value("recovery.epoch"),
            value("recovery.recipient_key_id"),
            value("recovery.payload_sha256"),
        ).joinToString("|")
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(canonical.toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }
        expect("recovery.binding_sha256", digest)

        val epochs = listOf(
            "epoch.initial",
            "epoch.after_add",
            "epoch.after_update_1",
            "epoch.after_update_2",
            "epoch.after_offline_join",
            "epoch.after_revoke",
        ).map { value(it).toInt() }
        require(epochs == (0..5).toList()) {
            "vector mismatch for epoch sequence: expected 0,1,2,3,4,5, got ${epochs.joinToString()}"
        }
    }
}

fun main(args: Array<String>) {
    require(args.size == 1) { "usage: kotlin harness <golden-vector>" }
    GoldenVector.load(Path.of(args.single())).validate()
    println("kotlin: PASS tl-mls-1 semantic golden vector")
}
