/* Warpnet - Decentralized Social Network */
package site.warpnet.transport

import com.squareup.moshi.Moshi
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/**
 * The node answers a plain success with `event.Accepted`, the literal
 * `{"code":0,"message":"Accepted"}` (warpnet/event/event.go). Treating that as
 * an error made every handler that returns it — bookmark, unfollow, block,
 * mute, mark-read, delete — report failure to the user while the mutation had
 * already been applied on the node.
 *
 * The contract these tests pin: the code alone decides. Errors always carry a
 * non-zero one (500 internal, 401 not-signed-in, 5000 node error, 5001 rate
 * limit); zero is success.
 */
class WarpnetClientErrorResponseTest {

    private val client = WarpnetClient(
        moshi = Moshi.Builder().build(),
        signer = NoOpSigner(),
        binding = UnusedBinding,
    )

    private fun assertAccepts(raw: String) {
        client.throwIfErrorResponse(raw)
    }

    private fun assertRejects(raw: String): WarpnetException.ProtocolError {
        try {
            client.throwIfErrorResponse(raw)
        } catch (e: WarpnetException.ProtocolError) {
            return e
        }
        fail("expected a ProtocolError for: $raw")
        error("unreachable")
    }

    @Test
    fun `event Accepted is success, not an error`() {
        assertAccepts("""{"code":0,"message":"Accepted"}""")
    }

    @Test
    fun `zero code is success whatever the message says`() {
        assertAccepts("""{"code":0,"message":"anything at all"}""")
        assertAccepts("""{"code":0,"message":""}""")
    }

    @Test
    fun `an internal node error is rejected`() {
        val e = assertRejects("""{"code":5000,"message":"token mismatch"}""")
        assertEquals(5000, e.code)
        assertEquals("token mismatch", e.serverMessage)
    }

    @Test
    fun `an unauthorized response is rejected`() {
        val e = assertRejects(
            """{"code":401,"message":"this connection is not signed in: log in on this node first"}"""
        )
        assertEquals(401, e.code)
    }

    @Test
    fun `a server error is rejected`() {
        val e = assertRejects("""{"code":500,"message":"boom"}""")
        assertEquals(500, e.code)
    }

    @Test
    fun `a rate limited response is still recognised as retryable`() {
        val e = assertRejects("""{"code":5001,"message":"rate limit exceeded"}""")
        assertTrue("5001 must stay retryable so request() backs off", e.isRateLimited)
    }

    @Test
    fun `a normal payload passes through`() {
        assertAccepts("""{"id":"01M1A3448WXQ79RBR6PGRD117W","username":"Claude"}""")
        assertAccepts("""{"cursor":"end","tweets":[]}""")
    }

    @Test
    fun `a payload carrying its own message field is not mistaken for an error`() {
        // A chat message DTO has a `message`, and without a `code` it must not
        // parse as a ResponseError.
        assertAccepts("""{"message":"hello there","chat_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV"}""")
    }

    @Test
    fun `an empty response is not an error`() {
        assertAccepts("")
    }

    @Test
    fun `a non-JSON body is a transport failure`() {
        try {
            client.throwIfErrorResponse("error: stream reset")
        } catch (e: WarpnetException.TransportFailure) {
            assertEquals("error: stream reset", e.message)
            return
        }
        fail("expected a TransportFailure for a non-JSON body")
    }
}

/**
 * The classification path never touches the binding; any call here means the
 * test drifted into code that talks to the Go host.
 */
private object UnusedBinding : WarpnetBinding {
    override fun initialize(
        privKeyHex: String,
        warpNetwork: String,
        pskHex: String,
        bootstrapNodes: String,
    ): String = error("binding must not be used")

    override fun connect(addrInfo: String): String = error("binding must not be used")
    override fun stream(protocolId: String, data: String): String = error("binding must not be used")
    override fun peerId(): String = error("binding must not be used")
    override fun isConnected(): Boolean = error("binding must not be used")
    override fun disconnect(): String = error("binding must not be used")
    override fun pause() = error("binding must not be used")
    override fun resume() = error("binding must not be used")
    override fun shutdown(): String = error("binding must not be used")
    override fun sign(body: String): String = error("binding must not be used")
    override fun refreshPeerAddrs(addrs: String): String = error("binding must not be used")
}
