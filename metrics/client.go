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
	network        string

	socksGauge  *prometheus.GaugeVec
	onlineGauge *prometheus.GaugeVec

	pusher *push.Pusher
}

func NewMetricsClient(pushGatewayURL string, network string) *MetricsClient {
	if network == "" {
		log.Fatalf("metrics: network is empty")
	}

	if pushGatewayURL == "" {
		log.Fatalf("metrics: push gateway url is empty")
	}
	onlineGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_online_status",
			Help: "1 if node is online, 0 otherwise",
		},
		[]string{"node_id", "node_type"},
	)

	socksGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "socks_active_connections",
			Help: "Current number of active SOCKS5 connections",
		},
		[]string{"node_id", "ip"},
	)
	pusher := push.New(pushGatewayURL, "warpnet_node").
		Grouping("network", network).
		Collector(onlineGauge).
		Collector(socksGauge)

	return &MetricsClient{
		pushGatewayURL: pushGatewayURL,
		jobName:        "warpnet_node",
		onlineGauge:    onlineGauge,
		socksGauge:     socksGauge,
		pusher:         pusher,
		network:        network,
	}
}

func (c *MetricsClient) PushStatusOnline(nodeId, nodeType string) {
	c.socksGauge.WithLabelValues(nodeId, nodeType).Set(1)
}

func (c *MetricsClient) PushSocksConnections(nodeId, ip string) {
	c.socksGauge.WithLabelValues(nodeId, ip).Set(1)
}

func (c *MetricsClient) RemoveSocksConnections(nodeId, ip string) {
	c.socksGauge.WithLabelValues(nodeId, ip).Set(0)
}
