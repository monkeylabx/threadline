import CryptoKit
import Foundation

enum HarnessError: Error, CustomStringConvertible {
    case usage
    case invalidLine(Int)
    case duplicateKey(String)
    case missing(String)
    case mismatch(key: String, expected: String, actual: String)

    var description: String {
        switch self {
        case .usage:
            return "usage: T011SwiftHarness <golden-vector>"
        case .invalidLine(let line):
            return "invalid vector line: \(line)"
        case .duplicateKey(let key):
            return "duplicate vector key: \(key)"
        case .missing(let key):
            return "missing vector key: \(key)"
        case .mismatch(let key, let expected, let actual):
            return "vector mismatch for \(key): expected \(expected), got \(actual)"
        }
    }
}

struct GoldenVector {
    let values: [String: String]

    init(path: String) throws {
        let source = try String(contentsOfFile: path, encoding: .utf8)
        var parsed: [String: String] = [:]
        for (offset, rawLine) in source.components(separatedBy: .newlines).enumerated() {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line.isEmpty || line.hasPrefix("#") { continue }
            guard let separator = line.firstIndex(of: "=") else {
                throw HarnessError.invalidLine(offset + 1)
            }
            let key = String(line[..<separator])
            let value = String(line[line.index(after: separator)...])
            guard !key.isEmpty, !value.isEmpty else {
                throw HarnessError.invalidLine(offset + 1)
            }
            guard parsed.updateValue(value, forKey: key) == nil else {
                throw HarnessError.duplicateKey(key)
            }
        }
        values = parsed
    }

    func value(_ key: String) throws -> String {
        guard let value = values[key] else { throw HarnessError.missing(key) }
        return value
    }

    func expect(_ key: String, _ expected: String) throws {
        let actual = try value(key)
        guard actual == expected else {
            throw HarnessError.mismatch(key: key, expected: expected, actual: actual)
        }
    }

    func validate() throws {
        try expect("format_version", "1")
        try expect("profile", "tl-mls-1")
        try expect("protocol", "MLS-1.0-RFC9420")
        try expect("ciphersuite", "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519")
        try expect("mls_library", "openmls")
        try expect("mls_library_version", "0.8.1")
        try expect("vector_class", "semantic-contract")
        try expect("recovery.wrapper", "optional")
        try expect("recovery.private_key_location", "external-kms-hsm-only")
        try expect("output.classification", "public-metadata-and-digests-only")

        let scenarios = [
            "key_package.expected": "accept",
            "offline_new_device.expected": "join-current-epoch",
            "out_of_order_commit.expected": "queue-until-predecessor",
            "device_revocation.expected": "removed-device-cannot-read-future-epoch",
            "history.authorized": "accept",
            "history.unauthorized": "TL_E2EE_HISTORY_UNAUTHORIZED",
            "history.cross_tenant": "TL_E2EE_TENANT_MISMATCH",
            "recovery.success": "accept",
            "recovery.no_recipient": "TL_E2EE_RECOVERY_UNAVAILABLE",
            "recovery.refused": "TL_E2EE_RECOVERY_REFUSED",
            "recovery.corrupt": "TL_E2EE_CORRUPT",
            "recovery.old_epoch": "TL_E2EE_OLD_EPOCH",
            "recovery.cross_group": "TL_E2EE_GROUP_MISMATCH",
            "recovery.cross_tenant": "TL_E2EE_TENANT_MISMATCH",
        ]
        for (key, expected) in scenarios {
            try expect(key, expected)
        }

        for key in [
            "error.replay", "error.corrupt", "error.old_epoch",
            "error.future_epoch", "error.unknown_version",
        ] {
            let code = try value(key)
            guard code.hasPrefix("TL_E2EE_") else {
                throw HarnessError.mismatch(
                    key: key,
                    expected: "stable TL_E2EE_* error",
                    actual: code
                )
            }
        }

        let canonical = [
            "tl-recovery-envelope-v\(try value("recovery.version"))",
            try value("recovery.tenant_id"),
            try value("recovery.group_id"),
            try value("recovery.epoch"),
            try value("recovery.recipient_key_id"),
            try value("recovery.payload_sha256"),
        ].joined(separator: "|")
        let digest = SHA256.hash(data: Data(canonical.utf8))
            .map { String(format: "%02x", $0) }
            .joined()
        try expect("recovery.binding_sha256", digest)

        let epochs = try [
            "epoch.initial", "epoch.after_add", "epoch.after_update_1",
            "epoch.after_update_2", "epoch.after_offline_join", "epoch.after_revoke",
        ].map { key -> Int in
            guard let epoch = Int(try value(key)) else {
                throw HarnessError.mismatch(key: key, expected: "integer epoch", actual: try value(key))
            }
            return epoch
        }
        guard epochs == Array(0...5) else {
            throw HarnessError.mismatch(
                key: "epoch sequence",
                expected: "0,1,2,3,4,5",
                actual: epochs.map(String.init).joined(separator: ",")
            )
        }
    }
}

do {
    guard CommandLine.arguments.count == 2 else { throw HarnessError.usage }
    let vector = try GoldenVector(path: CommandLine.arguments[1])
    try vector.validate()
    print("swift: PASS tl-mls-1 semantic golden vector")
} catch {
    FileHandle.standardError.write(Data("swift: FAIL \(error)\n".utf8))
    exit(1)
}
