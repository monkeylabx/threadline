import Foundation
import SwiftProtobuf

func fail(_ message: String) -> Never {
  FileHandle.standardError.write(Data("\(message)\n".utf8))
  exit(1)
}

func hexFile(_ url: URL) throws -> Data {
  let value = try String(contentsOf: url, encoding: .utf8)
    .trimmingCharacters(in: .whitespacesAndNewlines)
  guard value.count.isMultiple(of: 2) else { fail("invalid hex fixture: \(url.path)") }
  var output = Data()
  var index = value.startIndex
  while index < value.endIndex {
    let next = value.index(index, offsetBy: 2)
    guard let byte = UInt8(value[index..<next], radix: 16) else { fail("invalid hex fixture: \(url.path)") }
    output.append(byte)
    index = next
  }
  return output
}

do {
  let arguments = Array(CommandLine.arguments.dropFirst())
  guard arguments.count >= 4 else {
    fail("usage: compat produce|relay|consume GOLDEN_ROOT INPUT_ROOT OUTPUT_ROOT [EXPECTED_LABEL OUTPUT_LABEL]")
  }
  let mode = arguments[0]
  guard ["produce", "relay", "consume"].contains(mode) else { fail("unsupported mode: \(mode)") }
  let goldenRoot = URL(fileURLWithPath: arguments[1], isDirectory: true)
  let inputRoot = URL(fileURLWithPath: arguments[2], isDirectory: true)
  let outputRoot = URL(fileURLWithPath: arguments[3], isDirectory: true)
  let expectedLabel = arguments.count > 4 ? arguments[4] : ""
  let outputLabel = arguments.count > 5 ? arguments[5] : ""
  let sourceIsGolden = mode == "produce"
  let channelInput = try sourceIsGolden
    ? hexFile(inputRoot.appendingPathComponent("channel-event-envelope.golden.hex"))
    : Data(contentsOf: inputRoot.appendingPathComponent("channel.bin"))
  let recoveryInput = try sourceIsGolden
    ? hexFile(inputRoot.appendingPathComponent("recovery-envelope.golden.hex"))
    : Data(contentsOf: inputRoot.appendingPathComponent("recovery.bin"))
  let channelCanary = try hexFile(goldenRoot.appendingPathComponent("ciphertext-envelope.canary.hex"))
  let recoveryCanary = try hexFile(goldenRoot.appendingPathComponent("crypto-envelope.canary.hex"))
  guard channelInput.range(of: channelCanary) != nil else { fail("channel input dropped the exact field-50000 canary") }
  guard recoveryInput.range(of: recoveryCanary) != nil else { fail("recovery input dropped the exact field-50000 canary") }

  var channel = try Threadline_Message_V1_ChannelEventEnvelope(serializedBytes: channelInput)
  var recovery = try Threadline_Crypto_V1_RecoveryEnvelope(serializedBytes: recoveryInput)
  let expectedChannel = sourceIsGolden ? "evt-golden-0001" : expectedLabel
  let expectedRecovery = sourceIsGolden ? "recovery-key-golden-v7" : expectedLabel
  guard channel.eventID == expectedChannel else { fail("channel expected \(expectedChannel); got \(channel.eventID)") }
  guard recovery.recoveryKeyID == expectedRecovery else { fail("recovery expected \(expectedRecovery); got \(recovery.recoveryKeyID)") }

  if mode != "consume" {
    guard !outputLabel.isEmpty else { fail("\(mode) requires an output label") }
    channel.eventID = outputLabel
    recovery.recoveryKeyID = outputLabel
    let channelOutput = try channel.serializedData()
    let recoveryOutput = try recovery.serializedData()
    guard channelOutput.range(of: channelCanary) != nil else { fail("channel generated adapter dropped the exact field-50000 canary") }
    guard recoveryOutput.range(of: recoveryCanary) != nil else { fail("recovery generated adapter dropped the exact field-50000 canary") }
    try FileManager.default.createDirectory(at: outputRoot, withIntermediateDirectories: true)
    try channelOutput.write(to: outputRoot.appendingPathComponent("channel.bin"))
    try recoveryOutput.write(to: outputRoot.appendingPathComponent("recovery.bin"))
  }

  let displayedInput = expectedLabel.isEmpty ? "Golden Frames" : expectedLabel
  let suffix = outputLabel.isEmpty ? "" : " -> \(outputLabel)"
  print("Swift \(mode) passed for \(displayedInput)\(suffix).")
} catch {
  fail("Swift compatibility adapter failed: \(error)")
}
