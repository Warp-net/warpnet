package bootstrap

import "errors"

type codeHashesCache struct {
	hashes map[string]struct{}
}

const SelfHashConsensusKey = "selfhash"

// SelfHashExists reads only. No mutex needed
func (c *codeHashesCache) SelfHashExists(k, selfHashHex string) error {
	if c == nil {
		return errors.New("bootstrap: nil bootstrap hashes cache")
	}
	if k != SelfHashConsensusKey {
		return nil
	}

	if len(selfHashHex) == 0 {
		return errors.New("empty codebase hash")
	}
	if _, ok := c.hashes[selfHashHex]; ok {
		return nil
	}

	return errors.New("self hash wasn't recorded")
}
