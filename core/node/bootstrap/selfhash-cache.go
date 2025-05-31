package bootstrap

import (
	"errors"
	"github.com/Warp-net/warpnet/json"
)

type codeHashesCache struct {
	items map[string]struct{}
}

const SelfHashConsensusKey = "selfhash"

// ValidateSelfHashes reads only. No mutex needed
func (c *codeHashesCache) ValidateSelfHashes(k, selfHashObj string) error {
	if c == nil {
		return errors.New("nil code hashes cache")
	}
	if k != SelfHashConsensusKey {
		return nil
	}

	if len(selfHashObj) == 0 {
		return errors.New("empty codebase hashes")
	}

	var incomingSelfHashes = make(map[string]struct{})
	if err := json.JSON.Unmarshal([]byte(selfHashObj), &incomingSelfHashes); err != nil {
		return err
	}

	for h := range incomingSelfHashes {
		if _, ok := c.items[h]; ok {
			return nil
		}
	}

	return errors.New("self hash is not in the consensus records")
}
