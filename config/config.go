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

package config

import (
	"flag"
	"fmt"
	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	testNetNetwork = "testnet"
	warpnetNetwork = "warpnet"
)

const noticeTemplate = " %s version %s. Copyright (C) <%s> <%s>. This program comes with ABSOLUTELY NO WARRANTY; This is free software, and you are welcome to redistribute it under certain conditions.\n\n\n"

var warpnetBootstrapNodes = []string{
	"/ip4/207.154.221.44/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j",
	"/ip4/207.154.221.44/tcp/4002/p2p/12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU",
	"/ip4/207.154.221.44/tcp/4003/p2p/12D3KooWNXSGyfTuYc3JznW48jay73BtQgHszWfPpyF581EWcpGJ",
}

var testnetBootstrapNodes = []string{
	"/ip4/88.119.169.156/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j",
	"/ip4/88.119.169.156/tcp/4002/p2p/12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU",
	"/ip4/88.119.169.156/tcp/4003/p2p/12D3KooWNXSGyfTuYc3JznW48jay73BtQgHszWfPpyF581EWcpGJ",
}

var configSingleton config

func init() {
	pflag.String("database.dir", "storage", "Database directory name")
	pflag.String("server.host", "localhost", "Server host")
	pflag.String("server.port", "4002", "Server port")
	pflag.String("node.host.v4", "0.0.0.0", "Node host IPv4")
	pflag.String("node.host.v6", "::", "Node host IPv6")
	pflag.String("node.port", "4001", "Node port")
	pflag.String("node.seed", "", "Node seed for deterministic ID generation")
	pflag.String("node.network", "warpnet", "Private network. Use 'testnet' for testing env.")
	pflag.String("node.bootstrap", "", "Bootstrap nodes multiaddr list, comma separated")
	//pflag.String("node.metrics.server", "", "Metrics server address")
	pflag.String("node.moderator.modelname", "llama-2-7b-chat.Q8_0.gguf", "File name AI model. Unused if 'cid' provided")
	pflag.String("node.moderator.modelcid", "bafybeid7to3a6zkv5fdh5lw7iyl5wruj46qirvfsc6xbngprjy67ma6slm", "AI model content ID in IPFS. Unused if 'modelpath' provided")
	pflag.String("logging.level", "info", "Logging level")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	_ = viper.BindPFlags(pflag.CommandLine)

	bootstrapAddrList := make([]string, 0, len(warpnetBootstrapNodes))
	bootstrapAddrs := viper.GetString("node.bootstrap")

	split := strings.Split(bootstrapAddrs, ",")
	if bootstrapAddrs != "" {
		bootstrapAddrList = split
	}

	network := strings.TrimSpace(viper.GetString("node.network"))
	if network == "mainnet" {
		network = warpnetNetwork
	}
	if network == warpnetNetwork {
		bootstrapAddrList = append(bootstrapAddrList, warpnetBootstrapNodes...)
	}
	if network == testNetNetwork {
		bootstrapAddrList = append(bootstrapAddrList, testnetBootstrapNodes...)
	}

	version := root.GetVersion()
	fmt.Printf(noticeTemplate, strings.ToUpper(warpnet.WarpnetName), version, "2025", "Vadim Filin")

	host := viper.GetString("node.host.v4")
	port := viper.GetString("node.port")
	dbDir := viper.GetString("database.dir")

	seed := strings.TrimSpace(viper.GetString("node.seed"))
	if seed == "" {
		seed = "seed" + network + dbDir + host + port
	}
	appPath := getAppPath()

	modelPath := filepath.Join(appPath, strings.TrimSpace(viper.GetString("node.moderator.modelname")))
	dbPath := filepath.Join(appPath, strings.TrimSpace(network), strings.TrimSpace(dbDir))

	configSingleton = config{
		Version: semver.MustParse(strings.TrimSpace(string(version))),
		Node: node{
			Bootstrap: bootstrapAddrList,
			Seed:      seed,
			HostV4:    host,
			HostV6:    viper.GetString("node.host.v6"),
			Port:      port,
			Network:   network,
			Metrics: metrics{
				Server: viper.GetString("node.metrics.server"),
			},
			Moderator: moderator{
				Path: modelPath,
				CID:  viper.GetString("node.moderator.modelcid"),
			},
		},
		Database: database{
			Path: dbPath,
		},
		Server: server{
			Host: viper.GetString("server.host"),
			Port: viper.GetString("server.port"),
		},
		Logging: logging{Level: strings.TrimSpace(viper.GetString("logging.level"))},
	}
}

func Config() config {
	return configSingleton
}

type config struct {
	Version  *semver.Version
	Node     node
	Database database
	Server   server
	Logging  logging
}

type node struct {
	Bootstrap []string
	HostV4    string
	HostV6    string
	Port      string
	Network   string
	Metrics   metrics
	Moderator moderator
	Seed      string
}
type moderator struct {
	Path, CID string
}
type metrics struct {
	Server string
}
type database struct {
	Path string
}
type logging struct {
	Level  string
	Format string
}
type server struct {
	Host string
	Port string
}

func (n node) IsTestnet() bool {
	return n.Network == testNetNetwork
}

func (n node) AddrInfos() (infos []warpnet.WarpAddrInfo, err error) {
	for _, addr := range n.Bootstrap {
		maddr, err := warpnet.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}
		addrInfo, err := warpnet.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			return nil, err
		}
		infos = append(infos, *addrInfo)
	}
	return infos, nil
}

func getAppPath() string {
	var dbPath string

	switch runtime.GOOS {
	case "windows":
		// %LOCALAPPDATA% Windows
		appData := os.Getenv("LOCALAPPDATA") // C:\Users\{username}\AppData\Local
		if appData == "" {
			log.Fatal("failed to get path to LOCALAPPDATA")
		}
		dbPath = filepath.Join(appData, "warpdata")

	case "darwin", "linux", "android":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		dbPath = filepath.Join(homeDir, ".warpdata")

	default:
		log.Fatal("unsupported OS")
	}

	dbPath = filepath.Join(dbPath)

	err := os.MkdirAll(dbPath, 0750)
	if err != nil {
		log.Fatal(err)
	}

	return dbPath
}
