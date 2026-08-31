//nolint:all
package audit

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	mrand "math/rand"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

type stubEngine struct {
	ok     bool
	reason string
	err    error
}

func (e stubEngine) Moderate(string) (bool, string, error) {
	return e.ok, e.reason, e.err
}

func challengeFor(text string) Challenge {
	return Challenge{ChallengeID: "challenge-1", Text: text, ContentHash: ContentHash(text)}
}

func TestStreamChallengeHandler(t *testing.T) {
	t.Run("rejects a malformed payload", func(t *testing.T) {
		_, err := StreamChallengeHandler(stubEngine{}, nil)([]byte("{"), nil)
		require.Error(t, err)
	})

	t.Run("rejects an empty challenge", func(t *testing.T) {
		body, err := json.Marshal(Challenge{ChallengeID: "challenge-1"})
		require.NoError(t, err)

		_, err = StreamChallengeHandler(stubEngine{}, nil)(body, nil)
		require.ErrorIs(t, err, ErrEmptyChallenge)
	})

	t.Run("rejects a hash that does not bind the text", func(t *testing.T) {
		body, err := json.Marshal(Challenge{
			ChallengeID: "challenge-1", Text: "judge me", ContentHash: "not-the-hash",
		})
		require.NoError(t, err)

		_, err = StreamChallengeHandler(stubEngine{}, nil)(body, nil)
		require.ErrorIs(t, err, ErrChallengeHashMismatch)
	})

	t.Run("surfaces an engine failure", func(t *testing.T) {
		engineErr := errors.New("model unavailable")
		body, err := json.Marshal(challengeFor("judge me"))
		require.NoError(t, err)

		_, err = StreamChallengeHandler(stubEngine{err: engineErr}, nil)(body, nil)
		require.ErrorIs(t, err, engineErr)
	})

	t.Run("answers unsigned when no signer is wired", func(t *testing.T) {
		body, err := json.Marshal(challengeFor("judge me"))
		require.NoError(t, err)

		out, err := StreamChallengeHandler(stubEngine{ok: true, reason: "fine"}, nil)(body, nil)
		require.NoError(t, err)

		resp := out.(ChallengeResponse)
		require.Equal(t, "challenge-1", resp.ChallengeID)
		require.Equal(t, ContentHash("judge me"), resp.ContentHash)
		require.Equal(t, domain.OK, resp.Result)
		require.Empty(t, resp.Signature)
	})

	t.Run("signs the answer when a signer is wired", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		body, err := json.Marshal(challengeFor("judge me"))
		require.NoError(t, err)

		signer := NewResponseSigner(priv, "moderator-1", "llama-guard")
		out, err := StreamChallengeHandler(stubEngine{ok: false, reason: "violent"}, signer)(body, nil)
		require.NoError(t, err)

		resp := out.(ChallengeResponse)
		require.NotEmpty(t, resp.Signature)
		require.Equal(t, domain.ID("moderator-1"), resp.ModeratorID)
		require.Equal(t, domain.ModelType("llama-guard"), resp.Model)
	})
}

type stubStreamer struct {
	resp []byte
	err  error
}

func (s stubStreamer) GenericStream(string, stream.WarpRoute, any) ([]byte, error) {
	return s.resp, s.err
}

func TestStreamChallengerAsk(t *testing.T) {
	ch := challengeFor("judge me")

	t.Run("surfaces a transport failure", func(t *testing.T) {
		streamErr := errors.New("peer offline")
		_, err := NewStreamChallenger(stubStreamer{err: streamErr}).Ask("peer-1", ch)
		require.ErrorIs(t, err, streamErr)
	})

	t.Run("refuses to read an error envelope as an answer", func(t *testing.T) {
		body, err := json.Marshal(event.ResponseError{Message: "no thanks"})
		require.NoError(t, err)

		_, err = NewStreamChallenger(stubStreamer{resp: body}).Ask("peer-1", ch)
		require.ErrorIs(t, err, ErrChallengeRefused)
	})

	t.Run("surfaces a malformed answer", func(t *testing.T) {
		_, err := NewStreamChallenger(stubStreamer{resp: []byte("{")}).Ask("peer-1", ch)
		require.Error(t, err)
	})

	t.Run("decodes the answer", func(t *testing.T) {
		body, err := json.Marshal(ChallengeResponse{
			ChallengeID: ch.ChallengeID, ContentHash: ch.ContentHash, Result: domain.OK,
		})
		require.NoError(t, err)

		resp, err := NewStreamChallenger(stubStreamer{resp: body}).Ask("peer-1", ch)
		require.NoError(t, err)
		require.Equal(t, ch.ChallengeID, resp.ChallengeID)
	})
}

func TestStandingString(t *testing.T) {
	require.Equal(t, "probation", StandingProbation.String())
	require.Equal(t, "trusted", StandingTrusted.String())
	require.Equal(t, "suspect", StandingSuspect.String())
	require.Equal(t, "banned", StandingBanned.String())
	require.Equal(t, "unknown", Standing(99).String())
}

func TestLedgerIgnoresAnEmptyPeer(t *testing.T) {
	l := NewLedger()
	l.Record("", OutcomeCorrect)
	require.Empty(t, l.Snapshot())
}

func TestLedgerUnknownPeerIsOnProbation(t *testing.T) {
	require.Equal(t, StandingProbation, NewLedger().StandingOf("never-seen"))
}

func TestCorpusIgnoresEmptyText(t *testing.T) {
	c := NewCorpus()
	c.Remember("", domain.OK)
	safe, unsafe := c.Len()
	require.Zero(t, safe)
	require.Zero(t, unsafe)
}

func TestChallengeRandomPeerNeedsACorpus(t *testing.T) {
	a := NewAuditor("self", nil, NewLedger(), NewCorpus(), mrand.New(mrand.NewSource(1)))
	require.Nil(t, a.ChallengeRandomPeer([]string{"peer-1"}),
		"an empty corpus has nothing to ask about")
}
