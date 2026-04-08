package socks5

import (
	"context"
	crand "crypto/rand"
	"errors"
	log "github.com/sirupsen/logrus"
	"math/big"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	"io"
	"net"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
)

var telegramDCs = []string{
	"149.154.167.51:443",
	"149.154.167.91:443",
	"91.108.56.130:443",   // Asia
	"149.154.175.53:443",  // US
	"149.154.175.100:443", // US
}

func init() {
	var alive []string

	for _, addr := range telegramDCs {
		conn, err := dial(addr)
		if err != nil {
			continue
		}
		_ = conn.Close()
		alive = append(alive, addr)
	}
	log.Infof("Telegram DCs alive: %v", alive)

	if len(alive) == 0 {
		alive = append(alive, telegramDCs[0])
	}
	telegramDCs = alive
}

func StreamSocksExitHandler(s warpnet.WarpStream) {
	log.Debugf("socks5: exit node called: %s", s.Conn().RemoteMultiaddr())

	conn, err := dialAvailable()
	if err != nil {
		log.Errorf("socks5: failed to establish telegram connection: %v", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closeF := sync.OnceFunc(func() {
		cancel()
		_ = conn.Close()
		_ = s.Close()
	})
	defer closeF()

	log.Debugf("socks5: exit node connected to: %s", conn.RemoteAddr().String())

	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer closeF()

		_, err := io.Copy(conn, s)
		if isExpectedNetworkClose(err) {
			return nil
		}
		return err
	})
	g.Go(func() error {
		defer closeF()

		_, err := io.Copy(s, conn)
		if isExpectedNetworkClose(err) {
			return nil
		}
		return err
	})
	if err := g.Wait(); err != nil && !errors.Is(err, io.EOF) {
		log.Errorf("socks5: telegram server exited with error: %v", err)
	}
	log.Debugf("socks5: exit node job finished successfully")
}

const ErrTelegramUnreachable warpnet.WarpError = "no Telegram DC reachable"

func dialAvailable() (_ net.Conn, err error) {
	randomIndex, err := crand.Int(crand.Reader, big.NewInt(int64(len(telegramDCs))))
	if err != nil {
		return nil, ErrTelegramUnreachable
	}
	addr := telegramDCs[int(randomIndex.Int64())]
	conn, err := dial(addr)
	if err == nil {
		return conn, nil
	}

	for _, addr := range telegramDCs {
		conn, err := dial(addr)
		if err != nil {
			continue
		}
		return conn, nil
	}
	log.Debugf("failed to dial telegram connection: %v", err)
	return nil, ErrTelegramUnreachable
}

func dial(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second) // nolint: noctx
	if err != nil {
		log.Debugf("failed to dial telegram connection: %v, addr=[%s]", err, addr)
		return nil, ErrTelegramUnreachable
	}
	if tcpConnection, ok := conn.(*net.TCPConn); ok {
		_ = tcpConnection.SetKeepAlive(true)
		_ = tcpConnection.SetKeepAlivePeriod(30 * time.Second)
		_ = tcpConnection.SetNoDelay(true)
	}

	return conn, nil
}

func isExpectedNetworkClose(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	if strings.Contains(err.Error(), "stream reset") {
		return true
	}
	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		return true
	}
	return false
}
