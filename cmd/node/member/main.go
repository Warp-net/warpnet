package main

import (
	"os"
	"time"

	"github.com/Warp-net/warpnet"
	frontend "github.com/Warp-net/warpnet-frontend"
	"github.com/Warp-net/warpnet/config"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("panic: %v", r)
		}
	}()

	lvl, err := log.ParseLevel(config.Config().Logging.Level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
	if config.Config().Logging.Format == config.TextFormat {
		log.SetFormatter(&log.TextFormatter{TimestampFormat: time.DateTime})
	} else {
		log.SetFormatter(&log.JSONFormatter{TimestampFormat: time.DateTime})
	}
	log.SetOutput(os.Stdout)

	log.Infof("network: %s", config.Config().Node.Network)

	app := NewApp()
	icon := warpnet.GetLogo()
	setLinuxDesktopIcon(icon)

	err = wails.Run(&options.App{
		Title:            "warpnet",
		Width:            1024, //nolint:mnd
		Height:           1024, //nolint:mnd
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: frontend.GetStaticEmbedded(),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1}, //nolint:mnd
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
		Linux: &linux.Options{
			Icon:                icon,
			WindowIsTranslucent: false,
			WebviewGpuPolicy:    linux.WebviewGpuPolicyNever,
			ProgramName:         "warpnet",
		},
		Mac: &mac.Options{
			TitleBar:             nil,
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Preferences:          nil,
			DisableZoom:          false,
			About: &mac.AboutInfo{
				Title:   "warpnet",
				Message: "",
				Icon:    icon,
			},
			OnFileOpen: nil,
			OnUrlOpen:  nil,
		},
		Windows: &windows.Options{
			WebviewIsTransparent:                false,
			WindowIsTranslucent:                 false,
			DisableFramelessWindowDecorations:   false,
			Theme:                               windows.Dark,
			BackdropType:                        windows.Auto,
			Messages:                            nil,
			ResizeDebounceMS:                    0,
			OnSuspend:                           nil,
			OnResume:                            nil,
			WebviewGpuIsDisabled:                true,
			WebviewDisableRendererCodeIntegrity: false,
			EnableSwipeGestures:                 false,
			WindowClassName:                     "warpnet",
		},
	})
	if err != nil {
		log.Errorf("failed to start application: %s", err)
		return
	}
}
