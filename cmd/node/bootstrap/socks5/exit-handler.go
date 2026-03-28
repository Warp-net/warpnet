package socks5

import (
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	"io"
	"net"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
)

var telegramDCs = []string{
	// EU
	"149.154.167.50:443",
	"149.154.167.51:443",
	// ME
	"149.154.167.91:443",
	"149.154.167.92:443",
}

func StreamSocksExitHandler(s warpnet.WarpStream) {
	log.Debugf("socks5: exit node called: %s", s.Conn().RemoteMultiaddr())

	conn, err := dialFirstAvailable()
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

func dialFirstAvailable() (_ net.Conn, err error) {
	for _, addr := range telegramDCs {
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second) // nolint: noctx
		if err != nil {
			log.Errorf("failed to dial telegram connection: %v, addr=[%s]", err, addr)
			continue
		}
		if tcpConnection, ok := conn.(*net.TCPConn); ok {
			_ = tcpConnection.SetKeepAlive(true)
			_ = tcpConnection.SetKeepAlivePeriod(30 * time.Second)
			_ = tcpConnection.SetNoDelay(true)
		}

		return conn, nil
	}
	log.Errorf("failed to dial telegram connection: %v", err)
	return nil, ErrTelegramUnreachable
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
