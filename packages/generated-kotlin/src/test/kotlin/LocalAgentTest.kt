import com.threadline.proto.threadline.local_agent.v1.SubmitRunInputRequest
import com.threadline.proto.threadline.local_agent.v1.submitRunInputRequest
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals

class LocalAgentTest {
    @Test
    fun contractWireFixtureRoundtrips() {
        // test/fixtures/proto/agentd-local/wire.json: SubmitRunInputRequest.
        val wire = byteArrayOf(0x0a, 0x05) + "run-a".toByteArray(Charsets.UTF_8) +
            byteArrayOf(0x12, 0x07) + "input-a".toByteArray(Charsets.UTF_8) +
            byteArrayOf(0x1a, 0x06) + "héllo".toByteArray(Charsets.UTF_8)
        val request = SubmitRunInputRequest.parseFrom(wire)
        assertEquals("run-a", request.runId)
        assertEquals("input-a", request.inputId)
        assertEquals("héllo", request.text)
        assertContentEquals(wire, request.toByteArray())
        assertEquals(request, submitRunInputRequest {
            runId = "run-a"
            inputId = "input-a"
            text = "héllo"
        })
    }
}
