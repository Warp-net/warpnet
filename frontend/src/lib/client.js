// import base64 from 'base64-js';
// import diffieHellman from 'diffie-hellman';
// import {Buffer} from 'buffer';
// import {generateUUID} from "@/lib/uuid";
//
// let client;
//
// export function connectClient() {
//     if (client.socket && client.socket.readyState === "open") {
//         console.log("WebSocket is already connected.");
//         return;
//     }
//     client.close()
//     client = getClient()
//     client.connect()
// }
//
// export function getClient() {
//     if (!client) {
//         let host = window.backendHost;
//         let port = window.backendPort;
//         let address = "ws://" + host + ":" + port + "/v1/api/ws";
//         client = new EncryptedSocketClient(address);
//     }
//     return client;
// }
//
// // i hate frontend
// class EncryptedSocketClient {
//     constructor(url) {
//         this.dh = diffieHellman.getDiffieHellman('modp14'); // modp14 (2048 bits)
//         this.dh.generateKeys();
//         this.url = url;
//         this.publicKey = this.dh.getPublicKey();
//         this.externalPublicKey = null
//         this.aesKey = null; // Will be derived after receiving server's public key
//         this.socket = null
//         this.secretRenew = false;
//         this.messagesMap = new Map([
//             ["", {}], // request ID and response object
//         ]);
//     }
//
//     // Helper function to split message into ciphertext and nonce
//     splitMessage(message) {
//         let parts = message.split(":");
//         if (parts.length !== 2) {
//             console.error('error response:', message);
//             alert(message)
//         }
//         return parts
//     }
//
//     async deriveKey(sharedSecret, salt) {
//         const keyMaterial = await crypto.subtle.importKey(
//             'raw',
//             sharedSecret,
//             'HKDF',
//             false,
//             ['deriveKey']
//         );
//
//         return crypto.subtle.deriveKey(
//             {
//                 name: 'HKDF',
//                 salt: new TextEncoder().encode(salt),
//                 info: new Uint8Array(),
//                 hash: 'SHA-256',
//             },
//             keyMaterial,
//             { name: 'AES-GCM', length: 256 },
//             true,
//             ['encrypt', 'decrypt']
//         );
//     }
//
//     async encryptMessage(secretKey, plaintext) {
//         const iv = crypto.getRandomValues(new Uint8Array(12)); // 96-bit nonce
//         const ciphertext = await crypto.subtle.encrypt(
//             {
//                 name: 'AES-GCM',
//                 iv: iv,
//             },
//             secretKey,
//             new TextEncoder().encode(plaintext)
//         );
//         const encryptedResponse = Buffer.from(new Uint8Array(ciphertext));
//         const responseNonce = Buffer.from(iv);
//         return `${base64.fromByteArray(encryptedResponse)}:${base64.fromByteArray(responseNonce)}`;
//     }
//
//     async decryptMessage(secretKey, ciphertext, nonce) {
//         const decrypted = await crypto.subtle.decrypt(
//             {
//                 name: 'AES-GCM',
//                 iv: nonce,
//             },
//             secretKey,
//             ciphertext
//         );
//         return new TextDecoder().decode(decrypted);
//     }
//
//     async onMessage(event) {
//         const message = typeof event.data === "string" ? event.data : JSON.stringify(event.data);
//         const receivedMessage = Buffer.from(message, "base64");
//
//         // Handle the server's public key (first message)
//         if (!this.aesKey) {
//             console.log('Websocket received server public key');
//             this.externalPublicKey = receivedMessage
//             // Compute shared secret
//             const initSalt = getCurrentDate()
//             await this.computeSecret(initSalt) // temp salt
//             console.log("Websocket computed shared secret");
//             return;
//         }
//
//         // Process encrypted messages
//         const [ciphertextBase64, nonceBase64] = this.splitMessage(message);
//         const ciphertext = Buffer.from(ciphertextBase64, 'base64');
//         const nonce = Buffer.from(nonceBase64, 'base64');
//
//         // Decrypt the message
//         let plaintext = ''
//         try {
//             plaintext = await this.decryptMessage(this.aesKey, ciphertext, nonce);
//         } catch (err) {
//             console.error('Error decrypting message:', err.message);
//             return;
//         }
//
//
//         let resp = null
//         try {
//             resp = JSON.parse(plaintext);
//         } catch (error) {
//             console.error("JSON parsing:", plaintext, error.message);
//             return;
//         }
//
//         if (!resp) {
//             return;
//         }
//         if (!resp.body) {
//             console.error('Received invalid message structure - missing data value', plaintext);
//             return;
//         }
//         if (!resp.message_id) {
//             console.error('Received no message id for message:', resp);
//             return;
//         }
//
//         if (resp.body.identity && !this.secretRenew) {
//             await this.computeSecret(resp.body.identity.token); // renew salt to more secure one
//             this.secretRenew = true;
//             console.log('secret renewed')
//         }
//
//         this.messagesMap.set(resp.message_id, resp.body);
//     }
//
//     onError(error) {
//         console.error('WebSocket error:', error);
//     }
//
//     onClose() {
//         console.log('WebSocket connection closed');
//     }
//
//     close() {
//         if (this.socket) {
//             this.socket.removeEventListener('open', this.onOpen.bind(this));
//             this.socket.removeEventListener('close', this.onClose.bind(this));
//             this.socket.removeEventListener('message', this.onMessage.bind(this));
//             this.socket.removeEventListener('error', this.onError.bind(this));
//             this.socket.close();
//             this.socket = null;
//         }
//     }
//
//     async computeSecret(salt) {
//         const sharedSecret = this.dh.computeSecret(this.externalPublicKey);
//         // Derive AES key from the shared secret
//         this.aesKey = await this.deriveKey(sharedSecret, salt);
//     }
//
//      connect() {
//         try {
//             this.socket = new WebSocket(this.url);
//             this.socket.addEventListener('open', this.onOpen.bind(this));
//             this.socket.addEventListener('close', this.onClose.bind(this));
//             this.socket.addEventListener('message', this.onMessage.bind(this));
//             this.socket.addEventListener('error', this.onError.bind(this));
//         } catch (error) {
//             console.error("Error while connecting WebSocket:", error);
//         }
//     }
//
//     onOpen() {
//         console.log('WebSocket connection opened');
//         this.socket.send(this.publicKey.toString('base64'));
//         console.log('Client public key is sent.');
//     }
//
//     async getResponse(id) {
//         while (true) { // TODO think about it
//             const resp = this.messagesMap.get(id);
//             if (resp) {
//                 break;
//             }
//             await new Promise((r) => setTimeout(r, 1000));
//         }
//
//         let data = this.messagesMap.get(id)
//         this.messagesMap.delete(id)
//
//         return this.validateResponse(data);
//     }
//
//     validateResponse(data) {
//         if (data && data.code && data.code === 500) {
//             console.error(data.message);
//         }
//         return data
//     }
//
//     // user-only method, auth logic use 'this.socket.send'
//     async sendMessage(data) {
//         let id = generateUUID()
//         data.message_id = id
//
//         let message = JSON.stringify(data)
//
//
//         if (!this.aesKey) {
//             console.log("Sending message: not authenticated!");
//             return;
//         }
//         if (this.socket.readyState !== WebSocket.OPEN) {
//             console.error('WebSocket is not open');
//             return;
//         }
//         const payload = await this.encryptMessage(this.aesKey, message);
//         await this.socket.send(payload);
//         this.messagesMap.set(id, null); // empty response
//         return id
//     }
// }
//
// function getCurrentDate() {
//     const date = new Date();
//     return date.toISOString().split('T')[0];
// }
