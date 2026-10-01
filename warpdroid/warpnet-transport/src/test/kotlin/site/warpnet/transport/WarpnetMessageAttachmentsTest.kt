/* Warpnet - Decentralized Social Network */
package site.warpnet.transport

import com.squareup.moshi.Moshi
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import site.warpnet.transport.dto.WarpnetMessage

/**
 * Chat attachments reach the app as keys on the message, under the same wire
 * names the node writes (domain.ChatMessage). Dropping them here is what made
 * a picture message render as an empty bubble, so the shape is pinned: both
 * fields are optional, and a message without them still parses.
 */
class WarpnetMessageAttachmentsTest {

    private val moshi = Moshi.Builder().build()
    private val adapter = moshi.adapter(WarpnetMessage::class.java)

    @Test
    fun `parses attachment keys off the wire`() {
        val raw = """
            {"id":"m1","chat_id":"aaa:bbb","sender_id":"u1","receiver_id":"u2",
             "text":"look","created_at":"2026-09-18T15:00:00Z",
             "image_keys":["k1","k2"],"video_key":"v1"}
        """.trimIndent()

        val msg = adapter.fromJson(raw)!!

        assertEquals(listOf("k1", "k2"), msg.imageKeys)
        assertEquals("v1", msg.videoKey)
        assertEquals("u1", msg.senderId)
    }

    @Test
    fun `a message without attachments still parses`() {
        val raw = """
            {"id":"m2","chat_id":"aaa:bbb","sender_id":"u1","receiver_id":"u2",
             "text":"plain","created_at":"2026-09-18T15:00:00Z"}
        """.trimIndent()

        val msg = adapter.fromJson(raw)!!

        assertNull(msg.imageKeys)
        assertNull(msg.videoKey)
        assertEquals("plain", msg.text)
    }

    @Test
    fun `serialises back to the node's field names`() {
        val json = adapter.toJson(
            WarpnetMessage(
                id = "m3",
                chatId = "aaa:bbb",
                senderId = "u1",
                receiverId = "u2",
                text = "hi",
                imageKeys = listOf("k1"),
                videoKey = "v1",
            ),
        )

        assertTrue(json, json.contains("\"image_keys\""))
        assertTrue(json, json.contains("\"video_key\""))
    }
}
