/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.components.pairing

import javax.inject.Inject
import javax.inject.Singleton
import site.warpnet.transport.ConnectionState
import site.warpnet.transport.Ed25519IdentityStore
import site.warpnet.transport.WarpnetClient
import site.warpnet.transport.WarpnetConfig
import site.warpnet.transport.WarpnetException
import site.warpnet.transport.dto.AuthNodeInfo

/**
 * Runs the full pairing handshake: init → dial → pair → persist.
 *
 * Failure surfaces: [PairingOutcome.TransportError] for dial-time failures
 * (firewall, PSK mismatch, network unreachable) and
 * [PairingOutcome.Rejected] for server-side rejections ("token mismatch",
 * etc.). Identity is only persisted after the server returns exactly
 * `{"code":0,"message":"Accepted"}`.
 */
sealed class PairingOutcome {
    data class Success(val paired: PairedNode, val dialedAddr: String) : PairingOutcome()
    data class TransportError(val message: String) : PairingOutcome()
    data class Rejected(val code: Int, val message: String) : PairingOutcome()
    data class PeerIdMismatch(val expected: String, val errorMessage: String) : PairingOutcome()
}

@Singleton
class PairingCoordinator @Inject constructor(
    private val client: WarpnetClient,
    private val identityStore: Ed25519IdentityStore,
    private val pairedNodeStore: PairedNodeStore,
) {
    suspend fun pair(info: AuthNodeInfo, rawJson: String): PairingOutcome {
        val paired = PairedNode.from(info)
        // Build /p2p/<id>-terminated multiaddrs from NodeInfo.Addresses so the
        // Noise handshake verifies the remote peer ID matches what the QR
        // advertised. If they don't match, the dial fails before we open any
        // stream.
        val candidates = paired.addresses.map { "$it/p2p/${paired.pinnedPeerId}" }
        val bootstrap = paired.bootstrapAddrs.ifEmpty { candidates }
        // Seed the transport with the first syntactically-dialable candidate
        // rather than candidates.first(), which could still be an unusable
        // multiaddr when only a later entry passed the validator check.
        val dialable = candidates.firstOrNull { addr ->
            val bare = addr.substringBeforeLast("/p2p/")
            bare.startsWith("/") && bare.trim('/').isNotEmpty()
        } ?: candidates.first()

        val config = WarpnetConfig(
            privKeyHex = identityStore.loadOrCreateHex(),
            pskHex = paired.psk,
            bootstrapAddrs = bootstrap,
            desktopPeerAddr = dialable,
            network = paired.network,
        )

        return try {
            // Drop any prior host so a re-pair with a new PSK doesn't get
            // wedged on the gomobile singleton still holding the old private
            // network key.
            if (client.state.value != ConnectionState.Uninitialised) {
                client.shutdown()
            }
            client.initialise(config)
            val dialed = client.connectAny(candidates)
            client.pair(rawJson)
            pairedNodeStore.save(paired)
            PairingOutcome.Success(paired, dialed)
        } catch (e: WarpnetException.ProtocolError) {
            PairingOutcome.Rejected(e.code, e.serverMessage)
        } catch (e: WarpnetException.TransportFailure) {
            // libp2p verifies peer ID during the Noise handshake; a mismatch
            // surfaces as a transport failure with a "peer id mismatch" hint
            // from go-libp2p. Map it to a dedicated outcome so the UI can show
            // the right error.
            val msg = e.message.orEmpty()
            if (msg.contains("peer id mismatch", ignoreCase = true)) {
                PairingOutcome.PeerIdMismatch(paired.pinnedPeerId, msg)
            } else {
                PairingOutcome.TransportError(msg)
            }
        } catch (e: WarpnetException) {
            PairingOutcome.TransportError(e.message.orEmpty())
        }
    }
}
