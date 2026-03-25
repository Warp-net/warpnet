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
	"sync"
	"time"
)

type MetricsClient struct {
	pushGatewayURL            string
	jobName                   string
	network, nodeType, nodeID string

	socksGauge  *prometheus.GaugeVec
	onlineGauge *prometheus.GaugeVec

	pusher *push.Pusher

	once        sync.Once
	metricsChan chan any
	stopChan    chan struct{}
}

func NewMetricsClient(pushGatewayURL string, network, nodeType, nodeID string) *MetricsClient {
	if network == "" {
		log.Fatalf("metrics: network is empty")
	}
	if nodeType == "" {
		log.Fatalf("metrics: node type is empty")
	}
	if pushGatewayURL == "" {
		log.Fatalf("metrics: push gateway url is empty")
	}
	onlineGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_online_status",
			Help: "1 if node is online, 0 otherwise",
		},
		[]string{"network", "node_type"},
	)

	socksGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "socks_active_connections",
			Help: "Current number of active SOCKS5 connections",
		},
		[]string{"network", "ip"},
	)
	pusher := push.New(pushGatewayURL, "warpnet_node").
		Grouping("node_id", nodeID).
		Collector(onlineGauge).
		Collector(socksGauge)

	return &MetricsClient{
		pushGatewayURL: pushGatewayURL,
		jobName:        "warpnet_node",
		nodeType:       nodeType,
		onlineGauge:    onlineGauge,
		socksGauge:     socksGauge,
		pusher:         pusher,
		nodeID:         nodeID,
		network:        network,
		once:           sync.Once{},
		metricsChan:    make(chan any, 100),
		stopChan:       make(chan struct{}),
	}
}

func (c *MetricsClient) Start() {
	c.once.Do(func() {
		c.onlineGauge.WithLabelValues(c.network, c.nodeType).Set(1)

		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()

			// initial push
			if err := c.pusher.Push(); err != nil {
				log.Errorf("metrics: push failed: %v", err)
			}

			for {
				select {
				case <-c.stopChan:
					return
				case <-ticker.C:
					if err := c.pusher.Push(); err != nil {
						log.Errorf("metrics: push failed: %v", err)
					}
				}
			}
		}()
	})
}

func (c *MetricsClient) Stop() {
	close(c.stopChan)
}

func (c *MetricsClient) PushSocksConnections(ip string) {
	c.socksGauge.WithLabelValues(c.network, ip).Set(1)
}
