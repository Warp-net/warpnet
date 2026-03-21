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
		s.Write([]byte(err.Error()))
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
		s.Write([]byte(err.Error()))
	}
}

func dialFirstAvailable() (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}

	resultChan := make(chan result, len(telegramDCs))

	for _, addr := range telegramDCs {
		go func(address string) {
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			resultChan <- result{conn: conn, err: err}
		}(addr)
	}

	for i := 0; i < len(telegramDCs); i++ {
		res := <-resultChan
		if res.err == nil {
			return res.conn, nil
		}
	}

	return nil, fmt.Errorf("no DC reachable")
}
