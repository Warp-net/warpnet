package challenge

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/crypto/pb"
	log "github.com/sirupsen/logrus"
	"math/rand/v2"
	"time"
)

type SpoofChallenger struct {
	ctx     context.Context
	cache   *challengeCache
	retrier retrier.Retrier
}

func NewSpoofChallenger(ctx context.Context) *SpoofChallenger {
	return &SpoofChallenger{
		ctx:     ctx,
		cache:   newChallengeCache(),
		retrier: retrier.New(time.Second, 3, retrier.ExponentialBackoff),
	}
}

const (
	ErrChallengeMismatch         warpnet.WarpError = "challenge mismatch"
	ErrChallengeSignatureInvalid warpnet.WarpError = "invalid challenge signature"
)

type StreamFunc func(coord []event.ChallengeSample) ([]event.ChallengeSolution, error)

func (s *SpoofChallenger) Challenge(
	pubKey ic.PubKey, level int, streamF StreamFunc,
) (isRepeatable bool, err error) {
	if pubKey == nil {
		return false, warpnet.WarpError("public key is not found")
	}
	if pubKey.Type() != pb.KeyType_Ed25519 {
		return false, warpnet.WarpError("peer is not an Ed25519 public key")
	}

	id, rawPubKey := decomposePubKey(pubKey)
	if s.cache.IsChallengedAlready(id) {
		return false, nil
	}

	var (
		ownChallenges []challenge
		solutions     []event.ChallengeSolution
		coordinates   []event.ChallengeSample
	)
	if entry := s.cache.GetFailed(id); entry != nil {
		ownChallenges, coordinates = entry.challenge, entry.coordinates
		isRepeatable = true
	} else {
		ownChallenges, coordinates, err = s.composeChallengeRequest(level)
	}
	if err != nil {
		return isRepeatable, err
	}

	err = s.retrier.Try(s.ctx, func() error {
		solutions, err = streamF(coordinates)
		return err
	})
	if err != nil {
		s.cache.SetFailed(id, ownChallenges, coordinates)
		return isRepeatable, fmt.Errorf("failed to get challenge from new peer %s: %w", id, err)
	}

	if err := s.validateChallenges(ownChallenges, rawPubKey, solutions); err != nil {
		s.cache.SetFailed(id, ownChallenges, coordinates)
		return isRepeatable, err
	}

	s.cache.SetChallenged(id)
	s.cache.RemoveFailed(id)
	return false, nil
}

func (s *SpoofChallenger) composeChallengeRequest(challengeLevel int) ([]challenge, []event.ChallengeSample, error) {
	if challengeLevel == 0 {
		challengeLevel = 1
	}

	ownChalenges := make([][]byte, challengeLevel)
	coordinates := make([]event.ChallengeSample, challengeLevel)
	for i := 0; i < challengeLevel; i++ {
		nonce := rand.Int64() //#nosec
		ownChallenge, location, err := security.GenerateChallenge(root.GetCodeBase(), nonce)
		if err != nil {
			return nil, nil, err
		}
		ownChalenges[i] = ownChallenge
		coordinates[i] = event.ChallengeSample{
			DirStack:  location.DirStack,
			FileStack: location.FileStack,
			Nonce:     nonce,
		}
	}
	return ownChalenges, coordinates, nil
}

func (s *SpoofChallenger) validateChallenges(
	ownChallenges []challenge,
	rawPubKey ed25519.PublicKey,
	solutions []event.ChallengeSolution) error {
	if len(solutions) != len(ownChallenges) {
		err := warpnet.WarpError("invalid number of solutions")
		return fmt.Errorf("discovery: %w: %d != %d", err, len(solutions), len(ownChallenges))
	}

	for i := range solutions {
		ownChallenge := ownChallenges[i]
		solution := solutions[i]

		challengeRespDecoded, err := hex.DecodeString(solution.Challenge)
		if err != nil {
			return fmt.Errorf("failed to decode challenge origin: %w", err)
		}

		if !bytes.Equal(ownChallenge, challengeRespDecoded) {
			log.Errorf("challenge mismatch: %s != %s", hex.EncodeToString(ownChallenge), solution.Challenge)
			return ErrChallengeMismatch
		}

		if err := security.VerifySignature(rawPubKey, challengeRespDecoded, solution.Signature); err != nil {
			log.Errorf("invalid signature: %v", err)
			return ErrChallengeSignatureInvalid
		}
	}
	return nil
}

func decomposePubKey(pubKey ic.PubKey) (peerId string, raw ed25519.PublicKey) {
	rawPubKey, _ := pubKey.Raw()
	id, _ := warpnet.IDFromPublicKey(rawPubKey)
	return id.String(), rawPubKey
}
