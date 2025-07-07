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

package handlers

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/server/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type ClientNodeStreamer interface {
	ClientStream(nodeId string, path string, data any) (_ []byte, err error)
	IsRunning() bool
}

type AuthServicer interface {
	AuthLogin(message event.LoginEvent) (resp event.LoginResponse, err error)
	AuthLogout() error
}

type WSController struct {
	upgrader *websocket.EncryptedUpgrader
	auth     AuthServicer
	ctx      context.Context

	clientNode ClientNodeStreamer
}

func NewWSController(
	auth AuthServicer,
	clientNode ClientNodeStreamer,
) *WSController {

	return &WSController{nil, auth, nil, clientNode}
}

func (c *WSController) WebsocketUpgrade(ctx echo.Context) (err error) {
	log.Infof("websocket: upgrade request: %s", ctx.Request().URL.Path)

	c.upgrader = websocket.NewEncryptedUpgrader()
	c.upgrader.OnMessage(c.handle)

	c.ctx = ctx.Request().Context()

	err = c.upgrader.UpgradeConnection(ctx.Response(), ctx.Request()) // WS listener infinite loop
	if err != nil && !c.upgrader.IsCloseError(err) {
		log.Errorf("websocket: upgrader: %v", err)
	}

	c.upgrader.Close()
	c.upgrader = nil

	return nil
}

func (c *WSController) handle(msg []byte) (_ []byte, err error) {
	var (
		wsMsg    event.Message
		response event.Message
	)
	if err := json.JSON.Unmarshal(msg, &wsMsg); err != nil {
		return nil, err
	}
	if wsMsg.MessageId == "" {
		log.Errorf("websocket: request: missing message_id: %s\n", string(msg))
		return nil, fmt.Errorf("websocket: request: missing message_id")
	}
	if wsMsg.Body == nil {
		log.Errorf("websocket: request: missing body: %s\n", string(msg))
		return nil, fmt.Errorf("websocket: request: missing body")
	}

	switch wsMsg.Destination {
	case event.PRIVATE_POST_LOGIN:
		var ev event.LoginEvent
		err = json.JSON.Unmarshal(wsMsg.Body, &ev)
		if err != nil {
			log.Errorf("websocket: message body as login event: %v %s", err, wsMsg.Body)
			response = newErrorResp(err.Error())
			break
		}
		loginResp, err := c.auth.AuthLogin(ev)
		if err != nil {
			log.Errorf("websocket: auth: %v", err)
			response = newErrorResp(err.Error())
			break
		}
		c.upgrader.SetNewSalt(loginResp.Identity.Token) // make conn more secure after successful auth

		bt, err := json.JSON.Marshal(loginResp)
		if err != nil {
			log.Errorf("websocket: login FromLoginResponse: %v", err)
			break
		}
		msgBody := jsoniter.RawMessage(bt)
		response.Body = msgBody
	case event.PRIVATE_POST_LOGOUT:
		defer c.upgrader.Close()
		return nil, c.auth.AuthLogout()

	default:
		if c.clientNode == nil || !c.clientNode.IsRunning() {
			log.Errorf("websocket: request: not connected to server node")
			response = newErrorResp("not connected to server node")
			break
		}
		if wsMsg.Body == nil {
			log.Errorf("websocket: missing body: %s\n", string(msg))
			response = newErrorResp(fmt.Sprintf("missing data: %s", msg))
			break
		}

		if wsMsg.NodeId == "" || wsMsg.Destination == "" {
			log.Errorf("websocket: missing node id or path: %s\n", string(msg))
			response = newErrorResp(
				fmt.Sprintf("missing path or node ID: %s", msg),
			)
			break
		}

		log.Debugf("WS incoming message: %s %s\n", wsMsg.NodeId, stream.WarpRoute(wsMsg.Destination))
		respData, err := c.clientNode.ClientStream(wsMsg.NodeId, wsMsg.Destination, wsMsg.Body)
		if err != nil {
			log.Errorf("websocket: send stream: %v", err)
			response = newErrorResp(err.Error())
			break
		}
		msgBody := jsoniter.RawMessage(respData)
		response.Body = msgBody
	}
	if response.Body == nil {
		log.Errorf("websocket: response body is empty")
		return nil, nil
	}

	response.MessageId = wsMsg.MessageId
	response.NodeId = wsMsg.NodeId
	response.Destination = wsMsg.Destination
	response.Timestamp = time.Now()
	response.Version = config.Config().Version.String()

	var buffer bytes.Buffer
	err = json.JSON.NewEncoder(&buffer).Encode(response)
	return buffer.Bytes(), nil
}

func newErrorResp(message string) event.Message {
	errResp := event.ErrorResponse{
		Code:    http.StatusInternalServerError,
		Message: message,
	}

	bt, _ := json.JSON.Marshal(errResp)
	msgBody := jsoniter.RawMessage(bt)
	resp := event.Message{
		Body: msgBody,
	}
	return resp
}
