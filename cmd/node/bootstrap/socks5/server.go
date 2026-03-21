package socks5

import (
	"context"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/huandu/skiplist"
	"github.com/libp2p/go-libp2p/core/network"
	log "github.com/sirupsen/logrus"
	"github.com/things-go/go-socks5"
	"golang.org/x/sync/errgroup"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	defaultListenPort     = "1080"
	DefaultStreamProtocol = "/socks5/exit/1.0.0"
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
	if port == "" {
		port = defaultListenPort
	} else {
		port = port + "0"
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
	log.Infoln("socks5: psk is", psk)

	server := socks5.NewServer(
		socks5.WithLogger(log.StandardLogger()),
		socks5.WithConnectHandle(s.warpnetOverlayHandler),
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

func (s *socksServer) warpnetOverlayHandler(ctx context.Context, w io.Writer, r *socks5.Request) error {
	peer := s.pickSuitablePeer()
	stream, err := s.node.NewStream(ctx, peer, DefaultStreamProtocol)
	if err != nil {
		return err
	}
	defer func() {
		if err := stream.Close(); err != nil {
			log.Errorf("socks5: close stream: %v", err)
		}
	}()

	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.srv.Proxy(stream, r.Reader)
	})
	g.Go(func() error {
		return s.srv.Proxy(w, stream)
	})

	return g.Wait()
}

func (s *socksServer) pickSuitablePeer() warpnet.WarpPeerID {
	s.mx.RLock()
	defer s.mx.RUnlock()

	if s.latencyList.Len() == 0 {
		peers := s.node.Peerstore().PeersWithAddrs()
		if len(peers) != 0 {
			return peers[0]
		}
		return s.node.ID() // will fail with error "can't stream to yourself"
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
					if err := s.node.Connect(s.ctx, s.node.Peerstore().PeerInfo(peer)); err != nil {
						continue
					}
				}
				if p2pNet.Connectedness(peer) == network.CannotConnect {
					continue
				}
				latency := s.node.Peerstore().LatencyEWMA(peer)
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
