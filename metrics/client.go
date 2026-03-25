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
)

type MetricsClient struct {
	pushGatewayURL string
	jobName        string
	nodeID         string
}

func NewMetricsClient(pushGatewayURL string, nodeID string) *MetricsClient {
	return &MetricsClient{
		pushGatewayURL: pushGatewayURL,
		jobName:        "warpnet_node",
		nodeID:         nodeID,
	}
}

func (client *MetricsClient) PushStatusOnline(network string, nodeType string) {
	if client.pushGatewayURL == "" {
		return
	}
	if network == "" {
		log.Errorf("metrics: network is empty")
		return
	}

	onlineGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "node_online_status",
		Help: "1 if node is online, 0 otherwise",
		ConstLabels: prometheus.Labels{
			"network":   network,
			"node_type": nodeType,
		},
	})

	onlineGauge.Set(1)

	pusher := push.New(client.pushGatewayURL, client.jobName).
		Grouping("node_id", client.nodeID).
		Collector(onlineGauge)

	if err := pusher.Push(); err != nil {
		log.Errorf("metrics: push failed: %v", err)
	}
}
