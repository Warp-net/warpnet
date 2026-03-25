package socks5

import (
	"context"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	log "github.com/sirupsen/logrus"
	"github.com/things-go/go-socks5"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultListenPort                            = ":4080"
	DefaultStreamProtocol warpnet.WarpProtocolID = "/socks5/exit/1.0.0"
)

type Streamer interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
	Peerstore() warpnet.WarpPeerstore
	Network() warpnet.WarpNetwork
	SimpleConnect(warpnet.WarpAddrInfo) error
}

type MetricsPusher interface {
	PushSocksConnections(network string, value int64)
}

type socksServer struct {
	ctx      context.Context
	port     string
	connsNum atomic.Int64
	srv      *socks5.Server
	listener net.Listener
	streamer Streamer
	balancer *socksBalancer
	m        MetricsPusher
}

func NewServer(
	ctx context.Context,
	port, psk string,
	m MetricsPusher,
) *socksServer {
	if port == "" || !strings.HasPrefix(port, ":") {
		port = defaultListenPort
	}

	s := &socksServer{
		ctx:      ctx,
		port:     port,
		m:        m,
		connsNum: atomic.Int64{},
	}
	creds := socks5.StaticCredentials{"warpnet": psk}

	server := socks5.NewServer(
		socks5.WithDial(s.warpnetOverlayHandler),
		socks5.WithAuthMethods([]socks5.Authenticator{
			socks5.UserPassAuthenticator{Credentials: creds},
		}),
	)

	s.srv = server
	return s
}

func (s *socksServer) Start(streamer Streamer) error { // warpnet.P2PNode is libp2p host.Host alias
	l, err := net.Listen("tcp", s.port) // nolint: noctx
	if err != nil {
		return err
	}
	s.listener = l
	s.streamer = streamer
	s.balancer = newBalancer(s.ctx, streamer)
	go func() {
		if err := s.srv.Serve(l); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Errorf("socks5 server: serve failed: %v", err)
		}
	}()

	log.Infof("started socks5 server at %s", defaultListenPort)
	return nil
}

func (s *socksServer) Stop() error {
	log.Infof("stopped socks5 server at %s", defaultListenPort)
	s.balancer.Close()
	return s.listener.Close()
}

func (s *socksServer) warpnetOverlayHandler(ctx context.Context, net, addr string) (net.Conn, error) {
	stream, err := s.streamer.Node().NewStream(
		network.WithAllowLimitedConn(ctx, warpnet.WarpnetName),
		s.balancer.GetFastestPeer(),
		DefaultStreamProtocol,
	)
	if err != nil {
		return nil, fmt.Errorf("socks5: overlay stream: %w, address: %s", err, addr)
	}
	s.connsNum.Add(1)

	return &streamConn{
		once:   sync.Once{},
		stream: stream,
		customCloseF: func() {
			s.connsNum.Add(-1)
		},
	}, nil
}

type streamConn struct {
	once         sync.Once
	stream       warpnet.WarpStream
	customCloseF func()
}

func (c *streamConn) Read(p []byte) (int, error)  { return c.stream.Read(p) }
func (c *streamConn) Write(p []byte) (int, error) { return c.stream.Write(p) }
func (c *streamConn) Close() error {
	c.once.Do(c.customCloseF)
	return c.stream.Close()
}
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
