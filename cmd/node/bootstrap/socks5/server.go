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
	PushSocksConnections(nodeId, ip string)
	RemoveSocksConnections(nodeId, ip string)
}

type ctxKey string

const reqIpKey ctxKey = "ip-key"

type rule struct {
}

func (r *rule) Allow(ctx context.Context, req *socks5.Request) (context.Context, bool) {
	var host string

	switch addr := req.RemoteAddr.(type) {
	case *net.TCPAddr:
		host = addr.IP.String()
	default:
		host = req.RemoteAddr.String()
	}
	log.Debugf("socks5: request from %s", host)

	return context.WithValue(ctx, reqIpKey, host), true
}

type socksServer struct {
	ctx      context.Context
	port     string
	srv      *socks5.Server
	listener net.Listener
	nodeId   string
	streamer Streamer
	balancer *socksBalancer
	m        MetricsPusher
}

func NewServer(
	ctx context.Context,
	port, psk string,
	m MetricsPusher,
) *socksServer {
	if port == "" {
		port = defaultListenPort
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	s := &socksServer{
		ctx:  ctx,
		port: port,
		m:    m,
	}
	creds := socks5.StaticCredentials{warpnet.WarpnetName: psk}

	server := socks5.NewServer(
		socks5.WithRule(&rule{}),
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
	s.nodeId = s.streamer.Node().ID().String()
	log.Infof("started socks5 server at %s", s.port)
	return nil
}

func (s *socksServer) Stop() error {
	log.Infof("stopped socks5 server at %s", s.port)
	if s.balancer != nil {
		s.balancer.Close()
	}
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *socksServer) warpnetOverlayHandler(ctx context.Context, proto, addr string) (net.Conn, error) {
	host, _ := ctx.Value(reqIpKey).(string)
	s.m.PushSocksConnections(s.nodeId, host)
	removeMetrics := true
	defer func() {
		if removeMetrics {
			s.m.RemoveSocksConnections(s.nodeId, host)
		}
	}()

	peer, isRedirect := s.balancer.route()
	if peer == "" {
		return nil, fmt.Errorf("no peers found") //nolint:err113
	}
	if isRedirect {
		peerAddrs := s.streamer.Peerstore().Addrs(peer)
		log.Infof("socks5 server: redirect to %v", peerAddrs)
		for _, pAddr := range peerAddrs {
			dialer := net.Dialer{Timeout: 3 * time.Second}
			conn, err := dialer.DialContext(ctx, proto, toNetAddr(pAddr).String())
			if err != nil {
				continue
			}
			removeMetrics = false
			return &trackedConn{
				Conn: conn,
				closeF: func() {
					s.m.RemoveSocksConnections(s.nodeId, host)
				},
			}, nil
		}
	}
	log.Infof("socks5 server: stream to %s", peer.String())
	stream, err := s.streamer.Node().NewStream(
		network.WithAllowLimitedConn(ctx, warpnet.WarpnetName),
		peer,
		DefaultStreamProtocol,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"socks5: overlay stream: %w, proto: %s, address: %s",
			err, proto, addr,
		)
	}

	removeMetrics = false
	return &streamConn{
		stream: stream,
		closeF: func() {
			s.m.RemoveSocksConnections(s.nodeId, host)
		},
	}, nil
}

type streamConn struct {
	once   sync.Once
	stream warpnet.WarpStream
	closeF func()
}

type trackedConn struct {
	net.Conn

	once   sync.Once
	closeF func()
}

func (c *streamConn) Read(p []byte) (int, error)  { return c.stream.Read(p) }
func (c *streamConn) Write(p []byte) (int, error) { return c.stream.Write(p) }
func (c *streamConn) Close() error {
	c.once.Do(c.closeF)
	return c.stream.Close()
}
func (c *trackedConn) Close() error {
	c.once.Do(c.closeF)
	return c.Conn.Close()
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
