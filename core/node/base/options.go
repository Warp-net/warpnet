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

package base

import (
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p"
	libp2pConfig "github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	log "github.com/sirupsen/logrus"
	"reflect"
	"slices"
	"time"
	"unsafe"
)

func EnableAutoRelayWithStaticRelays(static []warpnet.WarpAddrInfo, currentNodeID warpnet.WarpPeerID) func() libp2p.Option {
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

func WithDialTimeout(t time.Duration) warpnet.SwarmOption {
	return func(s *warpnet.Swarm) error {
		return setPrivateDurationField(s, "dialTimeout", t)
	}
}

func WithDialTimeoutLocal(t time.Duration) warpnet.SwarmOption {
	return func(s *warpnet.Swarm) error {
		return setPrivateDurationField(s, "dialTimeoutLocal", t)
	}
}

func WithDefaultTCPConnectionTimeout(t time.Duration) warpnet.TCPOption {
	fieldName := "connectTimeout"
	return func(tr *warpnet.TCPTransport) error {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("reflection error setting %s: %v", fieldName, r)
			}
		}()
		v := reflect.ValueOf(tr).Elem()
		field := v.FieldByName(fieldName)
		if !field.IsValid() {
			return fmt.Errorf("field %s not found", fieldName)
		}
		if field.CanSet() {
			field.Set(reflect.ValueOf(t))
			return nil
		}
		ptr := unsafe.Pointer(field.UnsafeAddr())
		typedPtr := (*time.Duration)(ptr)
		*typedPtr = t
		return nil
	}
}

func setPrivateDurationField(s *warpnet.Swarm, fieldName string, t time.Duration) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("reflection error setting %s: %v", fieldName, r)
		}
	}()
	v := reflect.ValueOf(s).Elem()
	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return fmt.Errorf("field %s not found", fieldName)
	}
	if field.CanSet() {
		field.Set(reflect.ValueOf(t))
		return nil
	}
	ptr := unsafe.Pointer(field.UnsafeAddr())
	typedPtr := (*time.Duration)(ptr)
	*typedPtr = t
	return nil
}
