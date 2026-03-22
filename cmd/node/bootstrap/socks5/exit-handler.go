package socks5

import (
	"context"
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

	conn, err := dialFirstAvailable()
	if err != nil {
		log.Errorf("failed to establish telegram connection: %v", err)
		return
	}
	defer conn.Close()

	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() error {
		_, err := io.Copy(conn, s)
		return err
	})
	g.Go(func() error {
		_, err := io.Copy(s, conn)
		return err
	})

	if err := g.Wait(); err != nil {
		log.Errorf("telegram server exited with error: %v", err)
	}
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
