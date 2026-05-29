package main

import (
	"os"
	"time"

	"github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/cmd/node/member/deeplink"
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

	// Claim the warpnet:// URL scheme so OS-level clicks on
	// warpnet.site/user/{id} land back in this binary. Best-effort —
	// failure here only means deep links won't work, the rest of the
	// app keeps running. macOS is a no-op (declared in Info.plist).
	if err := deeplink.Register(); err != nil {
		log.Warnf("deeplink: scheme registration failed: %v", err)
	}

	// Capture a warpnet:// URL passed on argv (cold-start path: the
	// OS launched us with the link as an argument). We stash it on
	// the App and the frontend pulls it via ConsumePendingDeepLink
	// once it's ready to route.
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
			UniqueId: uuid.New().String(),
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				panic("second instance launched")
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
			// macOS routes warpnet:// clicks through this callback even
			// while the app is already running (single-process model
			// thanks to LaunchServices). We stash the raw URL and the
			// frontend picks it up via ConsumePendingDeepLink.
			OnUrlOpen: func(url string) {
				log.Infof("deeplink: macOS OnUrlOpen %s", url)
				app.SetPendingDeepLink(url)
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
