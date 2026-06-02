package main

import (
	"os"
	"time"

	"github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/cmd/node/member/deeplink"
	"github.com/Warp-net/warpnet/config"
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

	// Best-effort: failure just means deep links don't work.
	if err := deeplink.Register(); err != nil {
		log.Warnf("deeplink: scheme registration failed: %v", err)
	}

	if link, ok := deeplink.FromArgs(os.Args); ok {
		log.Infof("deeplink: cold-start link %s", link.Raw)
		app.SetPendingDeepLink(link.Raw)
	}

	err = wails.Run(&options.App{
		Title:            "warpnet", //nolint:goconst
		Width:            1024,
		Height:           1024,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: warpnet.GetStaticEmbedded(),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.close,
		SingleInstanceLock: &options.SingleInstanceLock{
			// Must be stable across launches — a fresh value per
			// start defeats the lock and lets every xdg-open spawn
			// a parallel process that fights for the Badger lock.
			UniqueId: "net.warpnet.app",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				if link, ok := deeplink.FromArgs(data.Args); ok {
					log.Infof("deeplink: second-instance link %s", link.Raw)
					app.NotifyDeepLink(link.Raw)
					return
				}
				log.Infof("deeplink: second-instance launch with args %v", data.Args)
			},
		},
		Bind: []any{
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
			// macOS hot-path: app stays single-process, URL clicks come here.
			OnUrlOpen: func(url string) {
				log.Infof("deeplink: macOS OnUrlOpen %s", url)
				app.NotifyDeepLink(url)
			},
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
