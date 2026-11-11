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

package warpnet

import (
	"crypto/ed25519"
	"fmt"
	camouflage "github.com/Warp-net/libp2p-camouflage-transport"
	"github.com/libp2p/go-libp2p/p2p/host/observedaddrs"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"reflect"
	"slices"
	"time"
	"unsafe"

	"github.com/libp2p/go-libp2p"
	libp2pConfig "github.com/libp2p/go-libp2p/config"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	log "github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

const defaultTimeout = 60 * time.Second

func init() {
	observedaddrs.ActivationThresh = 2 // TODO increase if there more than 1 user
}

var CommonOptions = []libp2p.Option{
	libp2p.WithDialTimeout(defaultTimeout),
	libp2p.SwarmOpts(
		WithDialTimeout(defaultTimeout),
		WithDialTimeoutLocal(defaultTimeout),
	),
	libp2p.Transport(camouflage.NewCamouflageTransport),
	libp2p.Ping(true),
	libp2p.Security(NoiseID, NewNoise),
	libp2p.EnableAutoNATv2(),
	libp2p.EnableRelay(),
	libp2p.EnableNATService(),
	libp2p.NATPortMap(),
	// the detector also refuses /ip6/ circuit addrs, though the relay may already be connected over IPv4
	libp2p.IPv6BlackHoleSuccessCounter(nil),
}

func NewEnableHolePunchingOption(tracer holepunch.EventTracer) WarpOption {
	return libp2p.EnableHolePunching(holepunch.WithTracer(tracer))
}

func NewWarpPeerstoreOption(ps WarpPeerstore) WarpOption {
	return libp2p.Peerstore(ps)
}

func NewWarpPrivateNetworkOption(psk PSK) WarpOption {
	return libp2p.PrivateNetwork(psk)
}

func NewListenAddrStringsOption(s ...string) WarpOption {
	return libp2p.ListenAddrStrings(s...)
}

func NewRoutingOption(cfg libp2pConfig.RoutingC) WarpOption {
	return libp2p.Routing(cfg)
}

func NewConnectionGaterOption(gater WarpConnectionGater) WarpOption {
	return libp2p.ConnectionGater(gater)
}

func NewSwarmInvokeOption(invoke func(WarpLifecycle, *Swarm)) WarpOption {
	return libp2p.WithFxOption(fx.Invoke(invoke))
}

func NewEventBusInvokeOption(invoke func(WarpEventBus)) WarpOption {
	return libp2p.WithFxOption(fx.Invoke(invoke))
}

func NewChainOption(opts ...WarpOption) WarpOption {
	return libp2p.ChainOptions(opts...)
}

var DefaultResources = relayv2.Resources{
	Limit: &relayv2.RelayLimit{
		Duration: DefaultRelayDurationLimit,
		Data:     DefaultRelayDataLimit,
	},

	ReservationTTL: time.Hour,

	MaxReservations: 128,
	MaxCircuits:     16,
	BufferSize:      4096,

	MaxReservationsPerIP:  8,
	MaxReservationsPerASN: 32,
}

const (
	DefaultRelayDataLimit     = 1 << 30 // 1 GiB
	DefaultRelayDurationLimit = time.Hour
)

func WithDefaultResourcesOption() relayv2.Option {
	return relayv2.WithResources(DefaultResources)
}

func NewConnManager(limiter rcmgr.Limiter, lowWater, highWater int) (*connmgr.BasicConnMgr, error) {
	high := min(highWater, limiter.GetSystemLimits().GetConnTotalLimit())
	return connmgr.NewConnManager(
		min(lowWater, high/2),
		high,
		connmgr.WithGracePeriod(time.Hour),
	)
}

func EnableAutoRelayWithStaticRelays(static []WarpAddrInfo, currentNodeID WarpPeerID) func() libp2p.Option {
	for i, info := range static {
		if info.ID == currentNodeID {
			static = slices.Delete(static, i, i+1)
			break
		}
	}
	return func() libp2p.Option {
		return func(cfg *libp2pConfig.Config) error {
			if len(static) == 0 {
				return nil
			}
			opts := []autorelay.Option{
				autorelay.WithBackoff(time.Minute * 5),
				autorelay.WithMaxCandidateAge(time.Minute * 5),
			}

			cfg.EnableAutoRelay = true
			cfg.AutoRelayOpts = append([]autorelay.Option{autorelay.WithStaticRelays(static)}, opts...)
			return nil
		}
	}
}

func EmptyOption() func() libp2p.Option {
	return func() libp2p.Option {
		return func(cfg *libp2pConfig.Config) error {
			return nil
		}
	}
}

const ErrMultipleIdentities WarpError = "cannot specify multiple identities"

func WarpIdentity(privKey ed25519.PrivateKey) libp2p.Option {
	p2pPrivKey, err := p2pCrypto.UnmarshalEd25519PrivateKey(privKey)
	if err != nil {
		panic(err)
	}
	return func(cfg *libp2pConfig.Config) error {
		if cfg.PeerKey != nil {
			return ErrMultipleIdentities
		}
		cfg.PeerKey = p2pPrivKey
		return nil
	}
}

func WithDialTimeout(t time.Duration) SwarmOption {
	return func(s *Swarm) error {
		return setPrivateDurationField(s, "dialTimeout", t)
	}
}

func WithDialTimeoutLocal(t time.Duration) SwarmOption {
	return func(s *Swarm) error {
		return setPrivateDurationField(s, "dialTimeoutLocal", t)
	}
}

const ErrFieldNotFound WarpError = "field not found"

func WithDefaultTCPConnectionTimeout(t time.Duration) TCPOption {
	fieldName := "connectTimeout"
	return func(tr *TCPTransport) error {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("options: reflection error setting %s: %v", fieldName, r)
			}
		}()
		v := reflect.ValueOf(tr).Elem()
		field := v.FieldByName(fieldName)
		if !field.IsValid() {
			return fmt.Errorf("options: %w: %s", ErrFieldNotFound, fieldName)
		}
		if field.CanSet() {
			field.Set(reflect.ValueOf(t))
			return nil
		}
		ptr := unsafe.Pointer(field.UnsafeAddr()) //#nosec
		typedPtr := (*time.Duration)(ptr)
		*typedPtr = t
		return nil
	}
}

func setPrivateDurationField(s *Swarm, fieldName string, t time.Duration) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("options: reflection error setting %s: %v", fieldName, r)
		}
	}()
	v := reflect.ValueOf(s).Elem()
	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return fmt.Errorf("options: %w: %s", ErrFieldNotFound, fieldName)
	}
	if field.CanSet() {
		field.Set(reflect.ValueOf(t))
		return nil
	}
	ptr := unsafe.Pointer(field.UnsafeAddr()) //#nosec
	typedPtr := (*time.Duration)(ptr)
	*typedPtr = t
	return nil
}
