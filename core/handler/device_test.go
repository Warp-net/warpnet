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

//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubDeviceRepo struct {
	aliases    []domain.Alias
	deleted    []string
	getAliasFn func() ([]domain.Alias, error)
	deleteFn   func(nodeId string) error
}

func (s *stubDeviceRepo) GetAliases() ([]domain.Alias, error) {
	if s.getAliasFn != nil {
		return s.getAliasFn()
	}
	return s.aliases, nil
}

func (s *stubDeviceRepo) DeleteAlias(nodeId string) error {
	if s.deleteFn != nil {
		if err := s.deleteFn(nodeId); err != nil {
			return err
		}
	}
	s.deleted = append(s.deleted, nodeId)
	return nil
}

func TestStreamGetDevicesHandler(t *testing.T) {
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db down")
		h := StreamGetDevicesHandler(&stubDeviceRepo{
			getAliasFn: func() ([]domain.Alias, error) { return nil, repoErr },
		})
		if _, err := h(nil, nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("paired devices", func(t *testing.T) {
		h := StreamGetDevicesHandler(&stubDeviceRepo{
			aliases: []domain.Alias{{NodeId: "device-1"}, {NodeId: "device-2"}},
		})
		resp, err := h(nil, nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		devices, ok := resp.(event.GetDevicesResponse)
		if !ok {
			t.Fatalf("expected GetDevicesResponse, got %T", resp)
		}
		if len(devices.Devices) != 2 {
			t.Fatalf("expected 2 devices, got %d", len(devices.Devices))
		}
	})
}

func TestStreamDeleteDeviceHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamDeleteDeviceHandler(&stubDeviceRepo{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty node id", func(t *testing.T) {
		h := StreamDeleteDeviceHandler(&stubDeviceRepo{})
		_, err := h(marshal(t, event.DeleteDeviceEvent{}), nil)
		if err == nil || err.Error() != "devices: empty node id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db down")
		h := StreamDeleteDeviceHandler(&stubDeviceRepo{
			deleteFn: func(string) error { return repoErr },
		})
		_, err := h(marshal(t, event.DeleteDeviceEvent{NodeId: "device-1"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("a paired device may only unpair itself", func(t *testing.T) {
		device := newAliasPeer(t)
		other := newAliasPeer(t)
		repo := &stubDeviceRepo{}
		h := StreamDeleteDeviceHandler(repo)

		s := pairedDeviceStream(t, event.PRIVATE_DELETE_DEVICE, newAliasPeer(t), device, true)
		_, err := h(marshal(t, event.DeleteDeviceEvent{NodeId: domain.ID(other.String())}), s)
		if err == nil || err.Error() != "devices: a device may only unpair itself" {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(repo.deleted) != 0 {
			t.Fatalf("expected nothing deleted, got %v", repo.deleted)
		}

		if _, err := h(marshal(t, event.DeleteDeviceEvent{NodeId: domain.ID(device.String())}), s); err != nil {
			t.Fatalf("expected a device to unpair itself, got: %v", err)
		}
	})

	t.Run("device unpaired", func(t *testing.T) {
		repo := &stubDeviceRepo{aliases: []domain.Alias{{NodeId: "device-2"}}}
		h := StreamDeleteDeviceHandler(repo)

		resp, err := h(marshal(t, event.DeleteDeviceEvent{NodeId: "device-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(repo.deleted) != 1 || repo.deleted[0] != "device-1" {
			t.Fatalf("expected device-1 deleted, got %v", repo.deleted)
		}
		devices, ok := resp.(event.GetDevicesResponse)
		if !ok {
			t.Fatalf("expected GetDevicesResponse, got %T", resp)
		}
		if len(devices.Devices) != 1 || devices.Devices[0].NodeId != "device-2" {
			t.Fatalf("expected the remaining device back, got %v", devices.Devices)
		}
	})
}
