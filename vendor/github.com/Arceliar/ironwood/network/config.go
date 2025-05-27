package network

import (
	"crypto/ed25519"
	"time"
)

type config struct {
	routerRefresh      time.Duration
	routerTimeout      time.Duration
	peerKeepAliveDelay time.Duration
	peerTimeout        time.Duration
	peerMaxMessageSize uint64
	bloomTransform     func(ed25519.PublicKey) ed25519.PublicKey
	pathNotify         func(ed25519.PublicKey)
	pathTimeout        time.Duration
	pathThrottle       time.Duration
}

type Option func(*config)

func configDefaults() Option {
	return func(c *config) {
		c.routerRefresh = 4 * time.Minute
		c.routerTimeout = 5 * time.Minute
		c.peerKeepAliveDelay = time.Second
		c.peerTimeout = 3 * time.Second
		c.peerMaxMessageSize = 1048576 // 1 megabyte
		c.bloomTransform = func(key ed25519.PublicKey) ed25519.PublicKey { return key }
		c.pathNotify = func(key ed25519.PublicKey) {}
		c.pathTimeout = time.Minute
		c.pathThrottle = time.Second
	}
}

func WithRouterRefresh(duration time.Duration) Option {
	return func(c *config) {
		c.routerRefresh = duration
	}
}

func WithRouterTimeout(duration time.Duration) Option {
	return func(c *config) {
		c.routerTimeout = duration
	}
}

func WithPeerKeepAliveDelay(duration time.Duration) Option {
	return func(c *config) {
		c.peerKeepAliveDelay = duration
	}
}

func WithPeerTimeout(duration time.Duration) Option {
	return func(c *config) {
		c.peerTimeout = duration
	}
}

func WithPeerMaxMessageSize(size uint64) Option {
	return func(c *config) {
		c.peerMaxMessageSize = size
	}
}

func WithBloomTransform(xform func(key ed25519.PublicKey) ed25519.PublicKey) Option {
	return func(c *config) {
		c.bloomTransform = xform
	}
}

func WithPathNotify(notify func(key ed25519.PublicKey)) Option {
	return func(c *config) {
		c.pathNotify = notify
	}
}

func WithPathTimeout(duration time.Duration) Option {
	return func(c *config) {
		c.pathTimeout = duration
	}
}

func WithPathThrottle(duration time.Duration) Option {
	return func(c *config) {
		c.pathThrottle = duration
	}
}
