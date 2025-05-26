package network

import (
	"crypto/ed25519"

	"github.com/Arceliar/ironwood/types"
)

const (
	publicKeySize  = ed25519.PublicKeySize
	privateKeySize = ed25519.PrivateKeySize
	signatureSize  = ed25519.SignatureSize
)

type publicKey [publicKeySize]byte
type privateKey [privateKeySize]byte
type signature [signatureSize]byte

type crypto struct {
	privateKey privateKey
	publicKey  publicKey
}

func (key *privateKey) sign(message []byte) signature {
	var sig signature
	tmp := ed25519.Sign(ed25519.PrivateKey(key[:]), message)
	copy(sig[:], tmp)
	return sig
}

func (key privateKey) equal(comparedKey privateKey) bool {
	return key == comparedKey
}

func (key *publicKey) verify(message []byte, sig *signature) bool {
	return ed25519.Verify(ed25519.PublicKey(key[:]), message, sig[:])
}

func (key publicKey) equal(comparedKey publicKey) bool {
	return key == comparedKey
}

func (key publicKey) less(comparedKey publicKey) bool {
	for idx := range key {
		switch {
		case key[idx] < comparedKey[idx]:
			return true
		case key[idx] > comparedKey[idx]:
			return false
		}
	}
	return false
}

func (key publicKey) addr() types.Addr {
	return types.Addr(key[:])
}

func (c *crypto) init(secret ed25519.PrivateKey) {
	copy(c.privateKey[:], secret)
	copy(c.publicKey[:], secret.Public().(ed25519.PublicKey))
}

func (key publicKey) toEd() ed25519.PublicKey {
	k := key
	return k[:]
}
