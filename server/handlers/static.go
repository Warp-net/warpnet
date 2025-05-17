/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package handlers

import (
	"bytes"
	"fmt"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/server/api-gen"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type StaticFolderOpener interface {
	Open(name string) (fs.File, error)
}

type StaticController struct {
	fileSystem               fs.FS
	isFirstRun               bool
	backendHost, backendPort string
}

func NewStaticController(
	isFirstRun bool,
	staticFolder StaticFolderOpener,
) *StaticController {
	pwd, _ := os.Getwd()
	log.Infoln("CURRENT DIRECTORY: ", pwd)
	fileSystem := echo.MustSubFS(staticFolder, "dist/")

	return &StaticController{
		fileSystem, isFirstRun,
		config.Config().Server.Host, config.Config().Server.Port,
	}
}

func (c *StaticController) GetIndex(ctx echo.Context) error {
	f, err := c.fileSystem.Open("index.html")
	if err != nil {
		log.Errorf("failed opening index.html: %v", err)
		return ctx.JSON(http.StatusInternalServerError, api.ErrorResponse{500, err.Error()})
	}
	fi, _ := f.Stat()

	content, err := io.ReadAll(f)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.ErrorResponse{500, err.Error()})
	}

	injectedContent := strings.Replace(
		string(content),
		"</head>",
		fmt.Sprintf(
			`<script>
			window.isFirstRun = %t;
			window.backendHost = %s;
			window.backendPort = %s;
		</script></head>`,
			c.isFirstRun,
			strconv.Quote(c.backendHost),
			strconv.Quote(c.backendPort),
		),
		1,
	)

	tempFile := bytes.NewReader([]byte(injectedContent))

	http.ServeContent(ctx.Response(), ctx.Request(), fi.Name(), fi.ModTime(), tempFile)
	return nil
}

func (c *StaticController) GetStaticFile(ctx echo.Context, filePath string) error {
	f, err := c.fileSystem.Open(filePath)
	if err != nil {
		f, err = c.fileSystem.Open("index.html")
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, api.ErrorResponse{500, err.Error()})
		}
	}
	fi, _ := f.Stat()
	ff := f.(io.ReadSeeker)

	http.ServeContent(ctx.Response(), ctx.Request(), fi.Name(), fi.ModTime(), ff)
	return nil
}
