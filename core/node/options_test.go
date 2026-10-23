//nolint:all
package node

import (
	"testing"

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
