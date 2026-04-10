package stream

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
)

func TestWarpRoute_ProtocolID(t *testing.T) {
	r := WarpRoute("/public/get/info/0.0.0")
	assert.Equal(t, warpnet.WarpProtocolID("/public/get/info/0.0.0"), r.ProtocolID())
}

func TestWarpRoute_String(t *testing.T) {
	r := WarpRoute("/public/get/info/0.0.0")
	assert.Equal(t, "/public/get/info/0.0.0", r.String())
}

func TestWarpRoute_IsPrivate(t *testing.T) {
	assert.True(t, WarpRoute("/private/post/login/0.0.0").IsPrivate())
	assert.False(t, WarpRoute("/public/get/info/0.0.0").IsPrivate())
}

func TestWarpRoute_IsGet(t *testing.T) {
	assert.True(t, WarpRoute("/public/get/info/0.0.0").IsGet())
	assert.False(t, WarpRoute("/public/post/like/0.0.0").IsGet())
}

func TestWarpRoutes_FromRoutesToPrIDs(t *testing.T) {
	routes := WarpRoutes{
		WarpRoute("/public/get/info/0.0.0"),
		WarpRoute("/private/post/login/0.0.0"),
	}
	prIDs := routes.FromRoutesToPrIDs()
	assert.Len(t, prIDs, 2)
	assert.Equal(t, warpnet.WarpProtocolID("/public/get/info/0.0.0"), prIDs[0])
	assert.Equal(t, warpnet.WarpProtocolID("/private/post/login/0.0.0"), prIDs[1])
}

func TestFromPrIDToRoute(t *testing.T) {
	prID := warpnet.WarpProtocolID("/public/get/info/0.0.0")
	r := FromPrIDToRoute(prID)
	assert.Equal(t, WarpRoute("/public/get/info/0.0.0"), r)
}

func TestFromProtocolIDToRoutes(t *testing.T) {
	prIDs := []warpnet.WarpProtocolID{
		"/public/get/info/0.0.0",
		"/private/post/login/0.0.0",
	}
	routes := FromProtocolIDToRoutes(prIDs)
	assert.Len(t, routes, 2)
	assert.Equal(t, WarpRoute("/public/get/info/0.0.0"), routes[0])
	assert.Equal(t, WarpRoute("/private/post/login/0.0.0"), routes[1])
}

func TestFromRoutesToPrIDs_Empty(t *testing.T) {
	routes := WarpRoutes{}
	prIDs := routes.FromRoutesToPrIDs()
	assert.Empty(t, prIDs)
}

func TestFromProtocolIDToRoutes_Empty(t *testing.T) {
	routes := FromProtocolIDToRoutes(nil)
	assert.Empty(t, routes)
}
