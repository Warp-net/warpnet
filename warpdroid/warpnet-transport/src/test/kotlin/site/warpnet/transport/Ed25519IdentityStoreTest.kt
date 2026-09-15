/* Warpnet - Decentralized Social Network */
package site.warpnet.transport

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/**
 * The paired node authorizes a device by its libp2p peer id alone, so the
 * key behind that peer id is a credential: two devices must never end up
 * with the same one, and a device must keep its own across restarts.
 */
class Ed25519IdentityStoreTest {

    // Stands in for one installation's storage: hands out whatever seed it
    // was given, as many times as it is asked.
    private class FixedSeeds(private val seed: ByteArray) : IdentitySeedStore {
        var reads = 0
        override fun seed(memberPeerId: String): ByteArray {
            reads++
            return seed
        }
    }

    private fun seedOf(fill: Byte) = ByteArray(32) { fill }

    @Test
    fun `two installations do not share a key`() {
        val one = Ed25519IdentityStore(FixedSeeds(seedOf(1))).derive(MEMBER)
        val other = Ed25519IdentityStore(FixedSeeds(seedOf(2))).derive(MEMBER)

        assertFalse(
            "distinct seeds must not produce the same identity",
            one.contentEquals(other),
        )
    }

    @Test
    fun `the same installation keeps its key`() {
        val store = Ed25519IdentityStore(FixedSeeds(seedOf(7)))

        assertArrayEquals(store.derive(MEMBER), store.derive(MEMBER))
    }

    @Test
    fun `key is the stored seed followed by its public key`() {
        val seed = seedOf(3)
        val key = Ed25519IdentityStore(FixedSeeds(seed)).derive(MEMBER)

        assertEquals(64, key.size)
        assertArrayEquals(seed, key.copyOfRange(0, 32))
    }

    @Test(expected = IllegalArgumentException::class)
    fun `an empty member peer id is rejected`() {
        Ed25519IdentityStore(FixedSeeds(seedOf(1))).derive("")
    }

    @Test(expected = IllegalArgumentException::class)
    fun `a seed of the wrong length is rejected`() {
        Ed25519IdentityStore(FixedSeeds(ByteArray(16))).derive(MEMBER)
    }

    private companion object {
        const val MEMBER = "12D3KooWQ3umNTQweTREML1gqyag4T2Ps82wLnHV7fUNQA8CnMa9"
    }
}
