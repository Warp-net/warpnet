package challenge

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
)

func TestNewChallengeCache(t *testing.T) {
	c := newChallengeCache()
	assert.NotNil(t, c)
	assert.NotNil(t, c.nodes)
	assert.NotNil(t, c.coords)
}

func TestIsChallengedAlready_NotChallenged(t *testing.T) {
	c := newChallengeCache()
	assert.False(t, c.IsChallengedAlready("peer1"))
}

func TestSetChallenged_ThenIsChallengedAlready(t *testing.T) {
	c := newChallengeCache()
	c.SetChallenged("peer1")

	// It might or might not be challenged depending on random wait period (0-8 hours)
	// Since the random could be 0 hours, we check the entry exists
	c.nodesMx.RLock()
	_, ok := c.nodes["peer1"]
	c.nodesMx.RUnlock()
	assert.True(t, ok, "entry should exist after SetChallenged")
}

func TestIsChallengedAlready_ExpiredChallenge(t *testing.T) {
	c := newChallengeCache()
	c.nodesMx.Lock()
	c.nodes["peer1"] = nodeEntry{
		nextChallenge: time.Now().Add(-time.Hour),
	}
	c.nodesMx.Unlock()

	assert.False(t, c.IsChallengedAlready("peer1"))
}

func TestIsChallengedAlready_FutureChallenge(t *testing.T) {
	c := newChallengeCache()
	c.nodesMx.Lock()
	c.nodes["peer1"] = nodeEntry{
		nextChallenge: time.Now().Add(time.Hour),
	}
	c.nodesMx.Unlock()

	assert.True(t, c.IsChallengedAlready("peer1"))
}

func TestSetChallenged_CleansExpiredEntries(t *testing.T) {
	c := newChallengeCache()
	// Add expired entry
	c.nodesMx.Lock()
	c.nodes["expired"] = nodeEntry{
		nextChallenge: time.Now().Add(-maxLiveTime - time.Hour),
	}
	c.nodesMx.Unlock()

	c.SetChallenged("new-peer")

	c.nodesMx.RLock()
	_, exists := c.nodes["expired"]
	c.nodesMx.RUnlock()
	assert.False(t, exists, "expired entry should be cleaned")
}

func TestGetFailed_NotSet(t *testing.T) {
	c := newChallengeCache()
	result := c.GetFailed("peer1")
	assert.Nil(t, result)
}

func TestSetFailed_ThenGetFailed(t *testing.T) {
	c := newChallengeCache()
	challenges := [][]byte{[]byte("ch1"), []byte("ch2")}
	coords := []event.ChallengeSample{
		{DirStack: []int{0}, FileStack: []int{0}, Nonce: 42},
	}

	c.SetFailed("peer1", challenges, coords)
	result := c.GetFailed("peer1")
	assert.NotNil(t, result)
	assert.Len(t, result.challenge, 2)
	assert.Len(t, result.coordinates, 1)
	assert.Equal(t, int64(42), result.coordinates[0].Nonce)
}

func TestRemoveFailed(t *testing.T) {
	c := newChallengeCache()
	c.SetFailed("peer1", [][]byte{[]byte("ch")}, nil)
	assert.NotNil(t, c.GetFailed("peer1"))

	c.RemoveFailed("peer1")
	assert.Nil(t, c.GetFailed("peer1"))
}

func TestRemoveFailed_Nonexistent(t *testing.T) {
	c := newChallengeCache()
	c.RemoveFailed("nonexistent")
	// Should not panic
}
