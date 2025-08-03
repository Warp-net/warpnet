package main

import (
	"fmt"
	frontend "github.com/Warp-net/warpnet-frontend"
	"github.com/Warp-net/warpnet/config"
	"github.com/google/uuid"
	writer "github.com/ipfs/go-log/writer"
	log "github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"os"
	"time"
)

func main() {
	defer closeWriter()
	lvl, err := log.ParseLevel(config.Config().Logging.Level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.DateTime,
	})
	log.SetOutput(os.Stdout)

	fmt.Println("network: ", config.Config().Node.Network)

	app := NewApp()

	err = wails.Run(&options.App{
		Title:            "warpnet",
		Width:            1024,
		Height:           1024,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: frontend.GetStaticEmbedded(),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.close,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: uuid.New().String(),
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				panic("second instance launched")
			},
		},
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		panic(fmt.Sprintf("failed to start application: %s", err))
	}
}

// TODO temp. Check for https://github.com/libp2p/go-libp2p-kad-dht/issues/1073
func closeWriter() {
	defer func() { recover() }()
	_ = writer.WriterGroup.Close()
}
