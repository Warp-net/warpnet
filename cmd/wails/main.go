package main

import (
	"fmt"
	frontend "github.com/Warp-net/warpnet-frontend"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"net/http"
	"os"
	"time"
)

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:              "Warpnet",
		Width:              1024,
		Height:             768,
		Frameless:          false,
		LogLevel:           logger.DEBUG,
		LogLevelProduction: logger.DEBUG,
		AssetServer: &assetserver.Options{
			Assets:     frontend.GetStaticEmbedded(),
			Middleware: nil,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		Bind: []interface{}{
			app,
		},
		Debug: options.Debug{OpenInspectorOnStartup: true},
	})

	if err != nil {
		log.Fatalln("error:", err.Error())
	}
}

type Message struct {
	Body        any       `json:"body"`
	MessageId   string    `json:"message_id"`
	NodeId      string    `json:"node_id"`
	Destination string    `json:"path"` // TODO change to 'destination'
	Timestamp   time.Time `json:"timestamp,omitempty"`
	Version     string    `json:"version"`
	Signature   string    `json:"signature"`
}

type App struct{}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

func (a *App) Route(wsMsg Message) any {
	fmt.Printf("MESSAGE!!!!! %v\n", wsMsg)

	var response Message

	if wsMsg.MessageId == "" {
		log.Errorf("websocket: request: missing message_id: %v\n", wsMsg)
		return fmt.Errorf("websocket: request: missing message_id")
	}
	if wsMsg.Body == nil {
		log.Errorf("websocket: request: missing body: %v\n", wsMsg)
		return fmt.Errorf("websocket: request: missing body")
	}

	switch wsMsg.Destination {
	case event.PRIVATE_POST_LOGIN:

		loginResp := event.LoginResponse{
			Identity: domain.Identity{
				Owner: domain.Owner{
					CreatedAt: time.Time{},
					NodeId:    "Node1",
					UserId:    "User1",
					Username:  "Vadim",
				},
				Token: "token",
			},
			NodeInfo: warpnet.NodeInfo{
				OwnerId:        "User1",
				ID:             "Node",
				Version:        nil,
				Addresses:      nil,
				StartTime:      time.Time{},
				RelayState:     "",
				BootstrapPeers: nil,
				Reachability:   1,
			},
		}

		bt, err := json.Marshal(loginResp)
		if err != nil {
			log.Errorf("websocket: login FromLoginResponse: %v", err)
			break
		}
		msgBody := json.RawMessage(bt)
		response.Body = msgBody
	case event.PRIVATE_POST_LOGOUT:
		//return nil, c.auth.AuthLogout()
		os.Exit(0)
		return nil
	default:
		//if c.clientNode == nil || !c.clientNode.IsRunning() {
		//	log.Errorf("websocket: request: not connected to server node")
		//	response = newErrorResp("not connected to server node")
		//	break
		//}
		if wsMsg.Body == nil {
			response = newErrorResp(fmt.Sprintf("missing data: %v", wsMsg))
			break
		}

		if wsMsg.NodeId == "" || wsMsg.Destination == "" {
			response = newErrorResp(
				fmt.Sprintf("missing path or node ID: %v", wsMsg),
			)
			break
		}

		log.Debugf("WS incoming message: %s %s\n", wsMsg.NodeId, stream.WarpRoute(wsMsg.Destination))
		tweetResp := event.TweetsResponse{
			Cursor: "cursor",
			Tweets: []domain.Tweet{{
				CreatedAt:   time.Time{},
				UpdatedAt:   nil,
				Id:          "1",
				ParentId:    nil,
				RetweetedBy: nil,
				RootId:      "",
				Text:        "test",
				UserId:      "Vadim",
				Username:    "",
				ImageKey:    "",
				Network:     "",
				Moderation:  nil,
			}},
			UserId: "",
		}

		respData, _ := json.Marshal(tweetResp)
		msgBody := json.RawMessage(respData)
		response.Body = msgBody
	}
	if response.Body == nil {
		return fmt.Errorf("websocket: response body is empty")
	}

	response.MessageId = wsMsg.MessageId
	response.NodeId = wsMsg.NodeId
	response.Destination = wsMsg.Destination
	response.Timestamp = time.Now()
	response.Version = "0.0.0"
	fmt.Printf("%v RESPONSE!", response.Body)
	return response
}

func newErrorResp(message string) Message {
	errResp := event.ErrorResponse{
		Code:    http.StatusInternalServerError,
		Message: message,
	}

	bt, _ := json.Marshal(errResp)
	msgBody := json.RawMessage(bt)
	resp := Message{
		Body: msgBody,
	}
	return resp
}
