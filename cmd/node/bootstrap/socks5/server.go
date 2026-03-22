package socks5

import (
	"context"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/huandu/skiplist"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	log "github.com/sirupsen/logrus"
	"github.com/things-go/go-socks5"
	"math/rand"
	"net"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	defaultListenPort                            = "4080"
	DefaultStreamProtocol warpnet.WarpProtocolID = "/socks5/exit/1.0.0"
)

type socksServer struct {
	ctx      context.Context
	port     string
	srv      *socks5.Server
	listener net.Listener
	node     warpnet.P2PNode

	mx               sync.RWMutex
	latencyList      *skiplist.SkipList
	peersWithLatency map[string]time.Duration

	stopChan chan struct{}
}

func NewServer(ctx context.Context, port, psk string) *socksServer {
	if port == "" || !strings.HasPrefix(port, ":") {
		port = defaultListenPort
	}

	s := &socksServer{
		ctx:              ctx,
		port:             port,
		latencyList:      skiplist.New(skiplist.Int64),
		stopChan:         make(chan struct{}),
		peersWithLatency: make(map[string]time.Duration),
		mx:               sync.RWMutex{},
	}
	creds := socks5.StaticCredentials{"warpnet": psk}

	server := socks5.NewServer(
		socks5.WithLogger(log.StandardLogger()),
		socks5.WithDial(s.warpnetOverlayHandler),
		socks5.WithAuthMethods([]socks5.Authenticator{
			socks5.UserPassAuthenticator{Credentials: creds},
		}),
	)

	s.srv = server
	return s
}

func (s *socksServer) Start(node warpnet.P2PNode) error { // warpnet.P2PNode is libp2p host.Host alias
	l, err := net.Listen("tcp", ":"+s.port) // nolint: noctx
	if err != nil {
		return err
	}
	s.listener = l
	s.node = node
	go func() {
		if err := s.srv.Serve(l); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Errorf("socks5 server: serve failed: %v", err)
		}
	}()

	go s.trackPeersLatency()
	log.Infof("started socks5 server at %s", defaultListenPort)
	return nil
}

func (s *socksServer) Stop() error {
	close(s.stopChan)
	log.Infof("stopped socks5 server at %s", defaultListenPort)
	return s.listener.Close()
}

func (s *socksServer) warpnetOverlayHandler(ctx context.Context, net, addr string) (net.Conn, error) {
	peer := s.pickSuitablePeer()
	log.Debugf("socks5: picked peer %s, %s, %s", peer.String(), net, addr)

	stream, err := s.node.NewStream(
		network.WithAllowLimitedConn(ctx, warpnet.WarpnetName),
		peer,
		DefaultStreamProtocol,
	)
	if err != nil {
		return nil, fmt.Errorf("socks5: overlay stream: %w, peer: %s", err, peer.String())
	}

	return &streamConn{stream}, nil
}

func (s *socksServer) pickSuitablePeer() warpnet.WarpPeerID {
	s.mx.RLock()
	defer s.mx.RUnlock()

	if s.latencyList.Len() == 0 {
		peers := s.node.Peerstore().PeersWithAddrs()
		if len(peers) != 0 {
			return peers[0]
		}
		return ""
	}

	for element := s.latencyList.Front(); element != nil; element = element.Next() {
		peer, ok := element.Value.(warpnet.WarpPeerID)
		if !ok {
			continue
		}
		return peer
	}
	pid, _ := s.latencyList.Front().Value.(warpnet.WarpPeerID)
	return pid
}

func (s *socksServer) trackPeersLatency() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			p2pStore := s.node.Peerstore()
			p2pNet := s.node.Network()

			peers := p2pStore.PeersWithAddrs()
			if len(peers) == 0 {
				time.Sleep(time.Second * 30)
				continue
			}
			for _, peer := range peers {
				if p2pNet.Connectedness(peer) == network.NotConnected {
					if err := s.node.Connect(s.ctx, p2pStore.PeerInfo(peer)); err != nil {
						continue
					}
				}
				if p2pNet.Connectedness(peer) == network.CannotConnect {
					continue
				}
				protocols, _ := p2pStore.GetProtocols(peer)
				if len(protocols) == 0 || !slices.Contains(protocols, DefaultStreamProtocol) {
					continue
				}

				latency := p2pStore.LatencyEWMA(peer)
				latency = latency + (time.Duration(rand.Intn(5)) * time.Millisecond) // jitter

				s.mx.Lock()
				if prevLatency, ok := s.peersWithLatency[peer.String()]; ok {
					s.latencyList.Remove(prevLatency.Milliseconds())
					delete(s.peersWithLatency, peer.String())
				}
				s.latencyList.Set(latency.Milliseconds(), peer)
				s.peersWithLatency[peer.String()] = latency
				s.mx.Unlock()
			}
		}
	}
}

type streamConn struct {
	stream warpnet.WarpStream
}

func (c *streamConn) Read(p []byte) (int, error)         { return c.stream.Read(p) }
func (c *streamConn) Write(p []byte) (int, error)        { return c.stream.Write(p) }
func (c *streamConn) Close() error                       { return c.stream.Close() }
func (c *streamConn) LocalAddr() net.Addr                { return toNetAddr(c.stream.Conn().LocalMultiaddr()) }
func (c *streamConn) RemoteAddr() net.Addr               { return toNetAddr(c.stream.Conn().RemoteMultiaddr()) }
func (c *streamConn) SetDeadline(t time.Time) error      { return c.stream.SetDeadline(t) }
func (c *streamConn) SetReadDeadline(t time.Time) error  { return c.stream.SetReadDeadline(t) }
func (c *streamConn) SetWriteDeadline(t time.Time) error { return c.stream.SetWriteDeadline(t) }

func toNetAddr(maddr multiaddr.Multiaddr) net.Addr {
	addr, err := manet.ToNetAddr(maddr)
	if err != nil {
		return &net.TCPAddr{} // fallback
	}
	return addr
}
