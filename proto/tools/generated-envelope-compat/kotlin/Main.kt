import com.threadline.proto.threadline.crypto.v1.RecoveryEnvelope
import com.threadline.proto.threadline.message.v1.ChannelEventEnvelope
import java.io.File

private fun fail(message: String): Nothing = error(message)

private fun hexFile(path: File): ByteArray =
    path.readText().trim().chunked(2).map { it.toInt(16).toByte() }.toByteArray()

private fun ByteArray.containsSequence(needle: ByteArray): Boolean =
    needle.isNotEmpty() && indices.any { start ->
        start + needle.size <= size && needle.indices.all { offset -> this[start + offset] == needle[offset] }
    }

fun main(args: Array<String>) {
    if (args.size < 4) {
        fail("usage: MainKt produce|relay|consume GOLDEN_ROOT INPUT_ROOT OUTPUT_ROOT [EXPECTED_LABEL OUTPUT_LABEL]")
    }
    val mode = args[0]
    if (mode !in setOf("produce", "relay", "consume")) fail("unsupported mode: $mode")
    val goldenRoot = File(args[1])
    val inputRoot = File(args[2])
    val outputRoot = File(args[3])
    val expectedLabel = args.getOrElse(4) { "" }
    val outputLabel = args.getOrElse(5) { "" }
    val sourceIsGolden = mode == "produce"

    val channelInput = if (sourceIsGolden) {
        hexFile(File(inputRoot, "channel-event-envelope.golden.hex"))
    } else {
        File(inputRoot, "channel.bin").readBytes()
    }
    val recoveryInput = if (sourceIsGolden) {
        hexFile(File(inputRoot, "recovery-envelope.golden.hex"))
    } else {
        File(inputRoot, "recovery.bin").readBytes()
    }
    val channelCanary = hexFile(File(goldenRoot, "ciphertext-envelope.canary.hex"))
    val recoveryCanary = hexFile(File(goldenRoot, "crypto-envelope.canary.hex"))
    if (!channelInput.containsSequence(channelCanary)) fail("channel input dropped the exact field-50000 canary")
    if (!recoveryInput.containsSequence(recoveryCanary)) fail("recovery input dropped the exact field-50000 canary")

    val channel = ChannelEventEnvelope.parseFrom(channelInput)
    val recovery = RecoveryEnvelope.parseFrom(recoveryInput)
    val expectedChannel = if (sourceIsGolden) "evt-golden-0001" else expectedLabel
    val expectedRecovery = if (sourceIsGolden) "recovery-key-golden-v7" else expectedLabel
    if (channel.eventId != expectedChannel) fail("channel expected $expectedChannel; got ${channel.eventId}")
    if (recovery.recoveryKeyId != expectedRecovery) fail("recovery expected $expectedRecovery; got ${recovery.recoveryKeyId}")

    if (mode != "consume") {
        if (outputLabel.isEmpty()) fail("$mode requires an output label")
        val channelOutput = channel.toBuilder().setEventId(outputLabel).build().toByteArray()
        val recoveryOutput = recovery.toBuilder().setRecoveryKeyId(outputLabel).build().toByteArray()
        if (!channelOutput.containsSequence(channelCanary)) fail("channel generated adapter dropped the exact field-50000 canary")
        if (!recoveryOutput.containsSequence(recoveryCanary)) fail("recovery generated adapter dropped the exact field-50000 canary")
        outputRoot.mkdirs()
        File(outputRoot, "channel.bin").writeBytes(channelOutput)
        File(outputRoot, "recovery.bin").writeBytes(recoveryOutput)
    }

    println("Kotlin $mode passed for ${expectedLabel.ifEmpty { "Golden Frames" }}${if (outputLabel.isEmpty()) "" else " -> $outputLabel"}.")
}
