//nolint:all
package warpnet

import (
	"testing"
	"time"

	libp2pConfig "github.com/libp2p/go-libp2p/config"
	"github.com/stretchr/testify/require"
)

// An IPv4-only node must keep dialing /ip6/ circuit addrs: the relay hop can
// reuse an existing IPv4 connection, so the IPv6 black hole detector stays off.
func TestCommonOptionsDisableIPv6BlackHoleDetector(t *testing.T) {
	var cfg libp2pConfig.Config
	require.NoError(t, cfg.Apply(CommonOptions...))
	require.True(t, cfg.CustomIPv6BlackHoleSuccessCounter)
	require.Nil(t, cfg.IPv6BlackHoleSuccessCounter)
}

func TestSetPrivateDurationFieldReportsMissingField(t *testing.T) {
	err := setPrivateDurationField(&Swarm{}, "thisFieldDoesNotExist", time.Second)
	require.ErrorIs(t, err, ErrFieldNotFound)
}

// The relay hands the same bootstrap slice to the DHT, so dropping itself must
// not rewrite the caller's list.
func TestEnableAutoRelayWithStaticRelaysKeepsCallerList(t *testing.T) {
	self := FromStringToPeerID("12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")
	other := FromStringToPeerID("12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU")
	static := []WarpAddrInfo{{ID: self}, {ID: other}}

	var cfg libp2pConfig.Config
	require.NoError(t, cfg.Apply(EnableAutoRelayWithStaticRelays(static, self)()))
	require.True(t, cfg.EnableAutoRelay)
	require.Equal(t, []WarpAddrInfo{{ID: self}, {ID: other}}, static)
}
