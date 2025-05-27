package mesh

import (
	"context"
	"encoding/hex"
	"fmt"
	config2 "github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/sirupsen/logrus"
	"net"
	"os"
	"regexp"
	"strings"
	"suah.dev/protect"
	"time"

	gologme "github.com/gologme/log"
	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/ipv6rwc"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	"github.com/yggdrasil-network/yggdrasil-go/src/tun"
)

var DefaultPeers = []string{
	"tls://88.119.169.156:7090",
	"tls://88.119.169.156:7092",
	"tls://88.119.169.156:7093",
}

type MeshLogger interface {
	Printf(s string, i ...interface{})
	Println(i ...interface{})
	Infof(s string, i ...interface{})
	Infoln(i ...interface{})
	Warnf(s string, i ...interface{})
	Warnln(i ...interface{})
	Errorf(s string, i ...interface{})
	Errorln(i ...interface{})
	Debugf(s string, i ...interface{})
	Debugln(i ...interface{})
	Traceln(i ...interface{})
}

type MeshRouter struct {
	ctx       context.Context
	core      *core.Core
	tun       *tun.TunAdapter
	multicast *multicast.Multicast
	admin     *admin.AdminSocket
	humanID   string
}

// NewMesh function is responsible for configuring and starting Yggdrasil.
func NewMeshRouter(
	ctx context.Context,
	libp2pBootstrapNodes []string,
	privKey []byte,
	l MeshLogger,
) (_ *MeshRouter, err error) {
	if err := protect.Unveil("/", "rwc"); err != nil {
		return nil, fmt.Errorf("unveil: / rwc: %v", err)
	}
	if err := protect.UnveilBlock(); err != nil {
		return nil, fmt.Errorf("unveil: %v", err)
	}

	ln := logrus.New()
	ln.SetLevel(logrus.DebugLevel)
	l = ln

	if len(libp2pBootstrapNodes) == 0 {
		libp2pBootstrapNodes = DefaultPeers
	}

	l.Infof("default peers: %v", libp2pBootstrapNodes)

	cfg := config.NodeConfig{
		PrivateKey: privKey,
		Peers:      libp2pBootstrapNodes,
		Listen: []string{
			fmt.Sprintf("tls://%s:%s", config2.Config().Mesh.Host, config2.Config().Mesh.Port),
		},
		AdminListen: "none",
		MulticastInterfaces: []config.MulticastInterfaceConfig{
			{Regex: ".*", Beacon: true, Listen: true},
		},
		IfName:          "none",
		LogLookups:      false,
		NodeInfoPrivacy: true,
		NodeInfo: map[string]interface{}{
			"TODO": nil, // TODO
		},
	}

	if err := cfg.GenerateSelfSignedCertificate(); err != nil {
		return nil, fmt.Errorf("mesh: generate self-signed certificate: %v", err)
	}

	n := &MeshRouter{ctx: ctx}

	iprange := net.IPNet{
		IP:   net.ParseIP("200::"),
		Mask: net.CIDRMask(7, 128),
	}

	options := []core.SetupOption{
		core.NodeInfo(cfg.NodeInfo),
		core.NodeInfoPrivacy(cfg.NodeInfoPrivacy),
		core.PeerFilter(func(ip net.IP) bool {
			return iprange.Contains(ip)
		}),
	}
	for _, addr := range cfg.Listen {
		options = append(options, core.ListenAddress(addr))
	}
	for _, peer := range cfg.Peers {
		options = append(options, core.Peer{URI: peer})
	}
	for intf, peers := range cfg.InterfacePeers {
		for _, peer := range peers {
			options = append(options, core.Peer{URI: peer, SourceInterface: intf})
		}
	}
	for _, allowed := range cfg.AllowedPublicKeys {
		k, err := hex.DecodeString(allowed)
		if err != nil {
			return nil, fmt.Errorf("mesh: hex: %v", err)
		}
		options = append(options, core.AllowedPublicKey(k[:]))
	}
	if n.core, err = core.New(cfg.Certificate, l, options...); err != nil {
		return nil, fmt.Errorf("mesh: core: %v", err)
	}

	//{
	//	// Set up the admin socket.
	//	options := []admin.SetupOption{
	//		admin.ListenAddress(cfg.AdminListen),
	//	}
	//	if cfg.LogLookups {
	//		options = append(options, admin.LogLookups{})
	//	}
	//	if n.admin, err = admin.New(n.core, logger, options...); err != nil {
	//		return nil, fmt.Errorf("mesh: admin: %v", err)
	//	}
	//	if n.admin != nil {
	//		n.admin.SetupAdminHandlers()
	//	}
	//}
	//// Set up the multicast module.
	{
		options := []multicast.SetupOption{}
		for _, intf := range cfg.MulticastInterfaces {
			options = append(options, multicast.MulticastInterface{
				Regex:    regexp.MustCompile(intf.Regex),
				Beacon:   intf.Beacon,
				Listen:   intf.Listen,
				Port:     intf.Port,
				Priority: uint8(intf.Priority),
				Password: intf.Password,
			})
		}

		logme := gologme.New(os.Stdout, "multicast: ", gologme.LstdFlags)
		logme.EnableLevel("debug")
		if n.multicast, err = multicast.New(n.core, logme, options...); err != nil {
			return nil, fmt.Errorf("mesh: multicast: %v", err)
		}

		//if n.admin != nil && n.multicast != nil {
		//	n.multicast.SetupAdminHandlers(n.admin)
		//}
	}

	// Set up the TUN module.
	{
		options := []tun.SetupOption{
			tun.InterfaceName(cfg.IfName),
			tun.InterfaceMTU(cfg.IfMTU),
		}
		if n.tun, err = tun.New(ipv6rwc.NewReadWriteCloser(n.core), l, options...); err != nil {
			return nil, fmt.Errorf("mesh: TUN: %v", err)
		}
		//if n.admin != nil && n.tun != nil {
		//	n.tun.SetupAdminHandlers(n.admin)
		//}
	}

	lp2pPubKey, err := warpnet.UnmarshalEd25519PublicKey(n.core.GetSelf().Key)
	if err != nil {
		return nil, fmt.Errorf("mesh: failed to unmarshal key: %v", err)
	}
	id, err := warpnet.IDFromPublicKey(lp2pPubKey)
	if err != nil {
		return nil, fmt.Errorf("mesh: failed to get human ID: %v", err)
	}
	n.humanID = id.String()

	println()
	fmt.Printf(
		"\033[1mMESH NETWORK LAYER INITIATED WITH ID %v AND ADDRESS %s\033[0m\n",
		id, n.core.Address().String(),
	)
	println()

	go func() {
		for {
			select {
			case <-n.ctx.Done():
				return
			default:
				for _, p := range n.core.GetPeers() {
					l.Infof("mesh peer %s status: %t", p.URI, p.Up)
				}
				for _, s := range n.core.GetSessions() {
					l.Infof("mesh session %s status: %d", s.Key, s.Uptime)
				}
				time.Sleep(time.Minute)
			}
		}
	}()

	//u, _ := url.Parse(DefaultPeer)

	return n, nil
}

func (mr *MeshRouter) HumanID() string {
	return mr.humanID
}

func (mr *MeshRouter) Address() net.IP {
	return mr.core.Address()
}

func (mr *MeshRouter) Subnet() net.IPNet {
	return mr.core.Subnet()
}

func (mr *MeshRouter) Stop() {
	if mr == nil {
		return
	}

	promises := []string{"stdio", "cpath", "inet", "unix", "dns"}
	if len(mr.multicast.Interfaces()) > 0 {
		promises = append(promises, "mcast")
	}
	if err := protect.Pledge(strings.Join(promises, " ")); err != nil {
		panic(fmt.Sprintf("pledge: %v: %v", promises, err))
	}

	if mr.admin != nil {
		_ = mr.admin.Stop()
	}
	if mr.multicast != nil {
		_ = mr.multicast.Stop()
	}
	if mr.tun != nil {
		_ = mr.tun.Stop()
	}
	if mr.core != nil {
		mr.core.Stop()
	}
}
