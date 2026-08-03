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
package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	helperEnvVar = "WARPNET_CONFIG_TEST_HELPER"
	helperMarker = "WARPNET_CONFIG_JSON "

	helperModeConfig = "config"
	helperModeFlags  = "flags"
)

type snapshot struct {
	HostV4         string   `json:"host_v4"`
	HostV6         string   `json:"host_v6"`
	Port           string   `json:"port"`
	Seed           string   `json:"seed"`
	Network        string   `json:"network"`
	Bootstrap      []string `json:"bootstrap"`
	MetricsGateway string   `json:"metrics_gateway"`
	IsPskPrinted   bool     `json:"is_psk_printed"`
	ModeratorPath  string   `json:"moderator_path"`
	ServerPort     string   `json:"server_port"`
	ServerPassword string   `json:"server_password"`
	DatabasePath   string   `json:"database_path"`
	LoggingLevel   string   `json:"logging_level"`
	LoggingFormat  string   `json:"logging_format"`
	IsTestnet      bool     `json:"is_testnet"`
	Version        string   `json:"version"`

	TestRunFlag string `json:"test_run_flag"`
}

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnvVar) {
	case helperModeConfig:
		c := Config()
		emit(snapshot{
			HostV4:         c.Node.HostV4,
			HostV6:         c.Node.HostV6,
			Port:           c.Node.Port,
			Seed:           c.Node.Seed,
			Network:        c.Node.Network,
			Bootstrap:      c.Node.Bootstrap,
			MetricsGateway: c.Node.Metrics.Gateway,
			IsPskPrinted:   c.Node.IsPskPrinted,
			ModeratorPath:  c.Node.Moderator.Path,
			ServerPort:     c.Node.Server.Port,
			ServerPassword: c.Node.Server.Password,
			DatabasePath:   c.Database.Path,
			LoggingLevel:   c.Logging.Level,
			LoggingFormat:  string(c.Logging.Format),
			IsTestnet:      c.Node.IsTestnet(),
			Version:        c.Version.String(),
		})
		os.Exit(0)

	case helperModeFlags:
		flag.Parse()
		var run string
		if f := flag.Lookup("test.run"); f != nil {
			run = f.Value.String()
		}
		emit(snapshot{TestRunFlag: run})
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func emit(s snapshot) {
	bt, err := json.Marshal(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper marshal:", err)
		os.Exit(1)
	}
	fmt.Println(helperMarker + string(bt))
}

func run(t *testing.T, mode string, args []string, env map[string]string) snapshot {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(self, args...) //#nosec
	cmd.Env = childEnv(mode, env)

	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "helper failed: %s", out)

	for _, line := range strings.Split(string(out), "\n") {
		if payload, ok := strings.CutPrefix(strings.TrimSpace(line), helperMarker); ok {
			var s snapshot
			require.NoError(t, json.Unmarshal([]byte(payload), &s))
			return s
		}
	}
	t.Fatalf("helper printed no configuration:\n%s", out)
	return snapshot{}
}

func childEnv(mode string, extra map[string]string) []string {
	managed := []string{
		"NODE_HOST_V4", "NODE_HOST_V6", "NODE_PORT", "NODE_SEED", "NODE_NETWORK",
		"NODE_BOOTSTRAP", "NODE_METRICS_GATEWAY", "NODE_PRINT_PSK",
		"NODE_MODERATOR_MODELPATH", "NODE_SERVER_PORT", "NODE_SERVER_PASSWORD",
		"LOGGING_LEVEL", "LOGGING_FORMAT", "DATABASE_DIR",
	}

	out := make([]string, 0, len(os.Environ())+len(extra)+1)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == helperEnvVar {
			continue
		}
		drop := false
		for _, m := range managed {
			if name == m {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	out = append(out, helperEnvVar+"="+mode)
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

func withFlags(t *testing.T, args ...string) snapshot {
	t.Helper()
	return run(t, helperModeConfig, args, nil)
}

func withEnv(t *testing.T, env map[string]string) snapshot {
	t.Helper()
	return run(t, helperModeConfig, nil, env)
}

func TestConfigInitDoesNotSwallowTestingFlags(t *testing.T) {
	got := run(t, helperModeFlags, []string{"-test.run=SomePattern"}, nil)
	assert.Equal(t, "SomePattern", got.TestRunFlag,
		"config init must leave flag.CommandLine unparsed for the testing package")
}

func TestConfigInitLeavesTestingFlagsEmptyWhenNotGiven(t *testing.T) {
	got := run(t, helperModeFlags, nil, nil)
	assert.Empty(t, got.TestRunFlag)
}

func TestDefaults(t *testing.T) {
	c := withFlags(t)

	assert.Equal(t, "0.0.0.0", c.HostV4)
	assert.Equal(t, "::", c.HostV6)
	assert.Equal(t, "4001", c.Port)
	assert.Equal(t, warpnetNetwork, c.Network)
	assert.Equal(t, "207.154.221.44:4091", c.MetricsGateway)
	assert.False(t, c.IsPskPrinted)
	assert.Equal(t, "/root/.warpdata/Llama-Guard-3-1B-Q4_K_M.gguf", c.ModeratorPath)
	assert.Equal(t, "4999", c.ServerPort)
	assert.Empty(t, c.ServerPassword, "the dashboard must not ship with a baked-in secret")
	assert.Equal(t, "info", c.LoggingLevel)
	assert.Equal(t, string(TextFormat), c.LoggingFormat)
	assert.False(t, c.IsTestnet)
	assert.NotEmpty(t, c.Version)
}

func TestDefaultSeedIsDerivedAndStable(t *testing.T) {
	first := withFlags(t)
	second := withFlags(t)

	assert.NotEmpty(t, first.Seed)
	assert.Equal(t, first.Seed, second.Seed, "the same settings must yield the same node identity")

	other := withFlags(t, "--node.port", "4002")
	assert.NotEqual(t, first.Seed, other.Seed, "a different port must not reuse the identity")

	assert.Contains(t, first.Seed, warpnetNetwork)
	assert.Contains(t, first.Seed, "4001")
}

func TestFlagsAreApplied(t *testing.T) {
	c := withFlags(t,
		"--node.host.v4", "127.0.0.1",
		"--node.host.v6", "::1",
		"--node.port", "4002",
		"--node.seed", "my-seed",
		"--node.network", testNetNetwork,
		"--node.metrics.gateway", "10.0.0.1:9091",
		"--node.print-psk",
		"--node.moderator.modelpath", "/models/guard.gguf",
		"--node.server.port", "5099",
		"--node.server.password", "s3cret",
		"--logging.level", "debug",
		"--logging.format", "json",
		"--database.dir", "mydb",
	)

	assert.Equal(t, "127.0.0.1", c.HostV4)
	assert.Equal(t, "::1", c.HostV6)
	assert.Equal(t, "4002", c.Port)
	assert.Equal(t, "my-seed", c.Seed)
	assert.Equal(t, testNetNetwork, c.Network)
	assert.Equal(t, "10.0.0.1:9091", c.MetricsGateway)
	assert.True(t, c.IsPskPrinted)
	assert.Equal(t, "/models/guard.gguf", c.ModeratorPath)
	assert.Equal(t, "5099", c.ServerPort)
	assert.Equal(t, "s3cret", c.ServerPassword)
	assert.Equal(t, "debug", c.LoggingLevel)
	assert.Equal(t, string(JSONFormat), c.LoggingFormat)
	assert.True(t, c.IsTestnet)
	assert.Contains(t, c.DatabasePath, filepath.Join(testNetNetwork, "mydb"))
}

func TestFlagsAcceptEqualsForm(t *testing.T) {
	c := withFlags(t, "--node.network=testnet", "--node.port=4444")

	assert.Equal(t, testNetNetwork, c.Network)
	assert.Equal(t, "4444", c.Port)
}

func TestEnvVarsAreApplied(t *testing.T) {
	c := withEnv(t, map[string]string{
		"NODE_HOST_V4":             "192.168.0.10",
		"NODE_HOST_V6":             "fe80::1",
		"NODE_PORT":                "4003",
		"NODE_SEED":                "env-seed",
		"NODE_NETWORK":             testNetNetwork,
		"NODE_METRICS_GATEWAY":     "10.0.0.2:9091",
		"NODE_PRINT_PSK":           "true",
		"NODE_MODERATOR_MODELPATH": "/env/guard.gguf",
		"NODE_SERVER_PORT":         "6000",
		"NODE_SERVER_PASSWORD":     "env-secret",
		"LOGGING_LEVEL":            "warn",
		"LOGGING_FORMAT":           "json",
		"DATABASE_DIR":             "envdb",
	})

	assert.Equal(t, "192.168.0.10", c.HostV4)
	assert.Equal(t, "fe80::1", c.HostV6)
	assert.Equal(t, "4003", c.Port)
	assert.Equal(t, "env-seed", c.Seed)
	assert.Equal(t, testNetNetwork, c.Network)
	assert.Equal(t, "10.0.0.2:9091", c.MetricsGateway)
	assert.True(t, c.IsPskPrinted)
	assert.Equal(t, "/env/guard.gguf", c.ModeratorPath)
	assert.Equal(t, "6000", c.ServerPort)
	assert.Equal(t, "env-secret", c.ServerPassword)
	assert.Equal(t, "warn", c.LoggingLevel)
	assert.Equal(t, string(JSONFormat), c.LoggingFormat)
	assert.True(t, c.IsTestnet)
	assert.Contains(t, c.DatabasePath, filepath.Join(testNetNetwork, "envdb"))
}

func TestDashedSettingIsReachableThroughEnv(t *testing.T) {
	c := withEnv(t, map[string]string{"NODE_PRINT_PSK": "true"})
	assert.True(t, c.IsPskPrinted)

	off := withEnv(t, map[string]string{"NODE_PRINT_PSK": "false"})
	assert.False(t, off.IsPskPrinted)
}

func TestEnvVarAloneDoesNotDisturbTheRest(t *testing.T) {
	c := withEnv(t, map[string]string{"NODE_NETWORK": testNetNetwork})

	assert.Equal(t, testNetNetwork, c.Network)
	assert.Equal(t, "4001", c.Port)
	assert.Equal(t, "0.0.0.0", c.HostV4)
	assert.Equal(t, "info", c.LoggingLevel)
}

func TestFlagOverridesEnv(t *testing.T) {
	c := run(t, helperModeConfig,
		[]string{"--node.network", testNetNetwork, "--node.port", "4002", "--logging.level", "debug"},
		map[string]string{
			"NODE_NETWORK":  warpnetNetwork,
			"NODE_PORT":     "9999",
			"LOGGING_LEVEL": "error",
		},
	)

	assert.Equal(t, testNetNetwork, c.Network)
	assert.Equal(t, "4002", c.Port)
	assert.Equal(t, "debug", c.LoggingLevel)
}

func TestMainnetAliasesToWarpnet(t *testing.T) {
	c := withFlags(t, "--node.network", "mainnet")

	assert.Equal(t, warpnetNetwork, c.Network)
	assert.False(t, c.IsTestnet)
	assert.Equal(t, warpnetBootstrapNodes, c.Bootstrap)
}

func TestBootstrapListFollowsTheNetwork(t *testing.T) {
	main := withFlags(t)
	assert.Equal(t, warpnetBootstrapNodes, main.Bootstrap)

	test := withFlags(t, "--node.network", testNetNetwork)
	assert.Equal(t, testnetBootstrapNodes, test.Bootstrap)

	assert.NotEqual(t, main.Bootstrap, test.Bootstrap,
		"a testnet node must not dial the production bootstrap nodes")
}

func TestCustomBootstrapIsAddedToTheDefaults(t *testing.T) {
	custom := "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"

	c := withFlags(t, "--node.bootstrap", custom)

	require.NotEmpty(t, c.Bootstrap)
	assert.Equal(t, custom, c.Bootstrap[0])
	assert.Subset(t, c.Bootstrap, warpnetBootstrapNodes)
}

func TestCustomBootstrapAcceptsACommaSeparatedList(t *testing.T) {
	a := "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	b := "/ip4/5.6.7.8/tcp/4002/p2p/12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU"

	c := withFlags(t, "--node.bootstrap", a+","+b)

	require.GreaterOrEqual(t, len(c.Bootstrap), 2)
	assert.Equal(t, a, c.Bootstrap[0])
	assert.Equal(t, b, c.Bootstrap[1])
}

func TestUnknownNetworkGetsNoBootstrapPeers(t *testing.T) {
	c := withFlags(t, "--node.network", "somethingelse")

	assert.Equal(t, "somethingelse", c.Network)
	assert.Empty(t, c.Bootstrap)
	assert.False(t, c.IsTestnet)
}

func TestDatabasePathIsSeparatedPerNetworkAndDir(t *testing.T) {
	main := withFlags(t)
	test := withFlags(t, "--node.network", testNetNetwork)

	assert.NotEqual(t, main.DatabasePath, test.DatabasePath)
	assert.Contains(t, main.DatabasePath, filepath.Join(warpnetNetwork, "storage"))
	assert.Contains(t, test.DatabasePath, filepath.Join(testNetNetwork, "storage"))

	custom := withFlags(t, "--database.dir", "second")
	assert.Contains(t, custom.DatabasePath, filepath.Join(warpnetNetwork, "second"))
	assert.NotEqual(t, main.DatabasePath, custom.DatabasePath)
}

func TestSurroundingWhitespaceIsTrimmed(t *testing.T) {
	c := withFlags(t,
		"--node.network", "  "+testNetNetwork+"  ",
		"--database.dir", "  spaced  ",
		"--logging.level", "  debug  ",
		"--node.seed", "  seeded  ",
		"--node.server.port", "  5099  ",
	)

	assert.Equal(t, testNetNetwork, c.Network)
	assert.Equal(t, "debug", c.LoggingLevel)
	assert.Equal(t, "seeded", c.Seed)
	assert.Equal(t, "5099", c.ServerPort)
	assert.Contains(t, c.DatabasePath, filepath.Join(testNetNetwork, "spaced"))
	assert.NotContains(t, c.DatabasePath, " ")
}

func TestLoggingFormatIsCaseInsensitive(t *testing.T) {
	c := withFlags(t, "--logging.format", "JSON")
	assert.Equal(t, string(JSONFormat), c.LoggingFormat)

	c = withFlags(t, "--logging.format", "Text")
	assert.Equal(t, string(TextFormat), c.LoggingFormat)
}

func TestIsTestnet(t *testing.T) {
	assert.True(t, node{Network: testNetNetwork}.IsTestnet())
	assert.False(t, node{Network: warpnetNetwork}.IsTestnet())
	assert.False(t, node{}.IsTestnet())
}

func TestAddrInfos(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		infos, err := node{}.AddrInfos()
		require.NoError(t, err)
		assert.Empty(t, infos)
	})

	t.Run("parses every peer", func(t *testing.T) {
		infos, err := node{Bootstrap: warpnetBootstrapNodes}.AddrInfos()
		require.NoError(t, err)
		require.Len(t, infos, len(warpnetBootstrapNodes))
		for _, info := range infos {
			assert.NotEmpty(t, info.ID.String())
			assert.NotEmpty(t, info.Addrs)
		}
	})

	t.Run("a malformed peer is reported, not skipped", func(t *testing.T) {
		_, err := node{Bootstrap: []string{"not-a-multiaddr"}}.AddrInfos()
		assert.Error(t, err)
	})

	t.Run("address without a peer id is rejected", func(t *testing.T) {
		_, err := node{Bootstrap: []string{"/ip4/1.2.3.4/tcp/4001"}}.AddrInfos()
		assert.Error(t, err)
	})
}

func TestBootstrapListsAreDistinctAndWellFormed(t *testing.T) {
	assert.NotEmpty(t, warpnetBootstrapNodes)
	assert.NotEmpty(t, testnetBootstrapNodes)

	for _, list := range [][]string{warpnetBootstrapNodes, testnetBootstrapNodes} {
		seen := make(map[string]struct{}, len(list))
		for _, addr := range list {
			_, dup := seen[addr]
			assert.Falsef(t, dup, "duplicate bootstrap entry %s", addr)
			seen[addr] = struct{}{}
			assert.Contains(t, addr, "/p2p/", "a bootstrap entry must name its peer")
		}
	}
}
