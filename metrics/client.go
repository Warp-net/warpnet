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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
	"strconv"
)

type metricsClient struct {
	address, nodeID string
	isBootstrapNode bool
}

func NewMetricsClient(
	address, nodeID string,
	isBootstrap bool,
) *metricsClient {
	return &metricsClient{address, nodeID, isBootstrap}
}

// PushStatusOnline - inform PushGateway about node status
func (m *metricsClient) PushStatusOnline() {
	if m.address == "" {
		return
	}
	if m.nodeID == "" {
		log.Errorf("failed to push status online, no node ID")
		return
	}
	onlineMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "node_online_status",
		Help: "1 if node is online, 0 otherwise.",
	})

	onlineMetric.Set(1)

	err := push.New(m.address, "status").
		Collector(onlineMetric).
		Grouping("node_id", m.nodeID).
		Grouping("is_bootstrap", strconv.FormatBool(m.isBootstrapNode)).
		Push()
	if err != nil {
		log.Errorf("failed to push online metric: %v", err)
	}
}
