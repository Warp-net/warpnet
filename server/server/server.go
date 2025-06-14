/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/server/api-gen"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/pkg/browser"
	log "github.com/sirupsen/logrus"
	"net/http"
)

var (
	ErrBrowserLoadFailed = errors.New("browser load failed")
)

type (
	Router            = api.EchoRouter
	HandlersInterface = api.ServerInterface
)

type PublicServer interface {
	Start()
	Router() Router
	Shutdown(ctx context.Context)
	RegisterHandlers(publicAPI HandlersInterface)
}

type interfaceServer struct {
	e    *echo.Echo
	port string
}

func NewInterfaceServer() (PublicServer, error) {
	conf := config.Config()
	swagger, err := api.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("loading swagger spec: %v", err)
	}
	swagger.Servers = nil
	swagger.Paths.Map()

	e := echo.New()
	e.HideBanner = true

	e.Use(echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
		AllowOrigins: []string{"localhost"}, // TODO
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
		AllowMethods: []string{http.MethodGet},
	}))
	e.Use(echomiddleware.Gzip())
	e.Use(secureHeadersMiddleware)

	port := ":" + conf.Server.Port
	err = browser.OpenURL("http://localhost" + port)
	if err != nil {
		log.Errorf("failed to open browser: %v", err)
		return nil, ErrBrowserLoadFailed
	}
	return &interfaceServer{e, port}, nil
}

func (p *interfaceServer) Start() {
	log.Infoln("starting public server...")
	err := p.e.Start(p.port)
	if err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			log.Infoln("public server stopped")
			return
		}
		log.Errorf("interface server start: %v", err)
	}
}

func (p *interfaceServer) RegisterHandlers(publicAPI HandlersInterface) {
	api.RegisterHandlers(p.e, publicAPI)
}

func (p *interfaceServer) Router() Router {
	return p.e
}

func (p *interfaceServer) Shutdown(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			p.e.Logger.Error(r)
		}
	}()
	log.Infoln("shutting down public server...")
	if p == nil {
		return
	}
	if p.e == nil {
		return
	}
	if err := p.e.Shutdown(ctx); err != nil {
		p.e.Logger.Error(err)
	}
}

func secureHeadersMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		if err := next(ctx); err != nil {
			ctx.Error(err)
		}
		ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
		ctx.Response().Header().Set("Pragma", "no-cache")
		ctx.Response().Header().Set("Expires", "0")
		ctx.Response().Header().Set("X-Content-Type-Options", "nosniff")
		ctx.Response().Header().Set("X-Frame-Options", "deny")
		ctx.Response().Header().Set("Content-Security-Policy", "default-src 'none'")
		ctx.Response().Header().Set("X-Powered-By", "")
		ctx.Response().Header().Set("Server", "")
		ctx.Response().Header().Set("Content-Type", "application/json")
		return nil
	}
}
