package network

import "crypto/ed25519"

type core struct {
	config config     // application-level configuration, must be the same on all nodes in a network
	crypto crypto     // crypto info, e.g. pubkeys and sign/verify wrapper functions
	router router     // logic to make next-hop decisions (plus maintain needed network state)
	peers  peers      // info about peers (from HandleConn), makes routing decisions and passes protocol traffic to relevant parts of the code
	pconn  PacketConn // net.PacketConn-like interface
}

func (c *core) init(secret ed25519.PrivateKey, opts ...Option) error {
	opts = append([]Option{configDefaults()}, opts...)
	for _, opt := range opts {
		opt(&c.config)
	}
	c.crypto.init(secret)
	c.router.init(c)
	c.peers.init(c)
	c.pconn.init(c)
	return nil
}
