package socks5

import (
	"context"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"

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
	defer s.Close()
	log.Infof("socks5: exit node called: %s", s.Conn().RemoteMultiaddr())

	conn, err := dialFirstAvailable()
	if err != nil {
		log.Errorf("failed to establish telegram connection: %v", err)
		return
	}
	defer conn.Close()

	log.Infof("socks5: exit node connected to: %s", conn.RemoteAddr().String())

	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() error {
		_, err := io.Copy(conn, s)
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
		return err
	})
	g.Go(func() error {
		_, err := io.Copy(s, conn)
		_ = s.CloseWrite()
		return err
	})

	if err := g.Wait(); err != nil && !errors.Is(err, io.EOF) {
		log.Errorf("telegram server exited with error: %v", err)
	}
	log.Infof("socks5: exit node job finished successfully")
}

func dialFirstAvailable() (net.Conn, error) {
	for _, addr := range telegramDCs {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			log.Errorf("failed to dial telegram connection: %v, addr=[%s]", err, addr)
			continue
		}
		return conn, nil
	}

	return nil, fmt.Errorf("no DC reachable")
}
