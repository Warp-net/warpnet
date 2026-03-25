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
	return &MetricsClient{
		pushGatewayURL: pushGatewayURL,
		jobName:        "warpnet_node",
		nodeType:       nodeType,
		nodeID:         nodeID,
		network:        network,
		once:           sync.Once{},
		metricsChan:    make(chan any, 100),
		stopChan:       make(chan struct{}),
	}
}

func (c *MetricsClient) Stop() {
	close(c.stopChan)
}

func (c *MetricsClient) PushStatusOnline() {
	c.once.Do(func() {
		onlineGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "node_online_status",
			Help: "1 if node is online, 0 otherwise",
			ConstLabels: prometheus.Labels{
				"network":   c.network,
				"node_type": c.nodeType,
			},
		})
		onlineGauge.Set(1)

		go c.push(onlineGauge)
	})
}

func (c *MetricsClient) push(collector prometheus.Collector) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	pusher := push.New(c.pushGatewayURL, c.jobName).
		Grouping("node_id", c.nodeID).
		Collector(collector)

	if err := pusher.Push(); err != nil {
		log.Errorf("metrics: push failed: online %v", err)
	}

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if err := pusher.Push(); err != nil {
				log.Errorf("metrics: push failed: online %v", err)
			}
		}
	}
}

func (c *MetricsClient) PushSocksConnections(ip string, value int64) {
	socksGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "socks_active_connections",
		Help: "Current number of active SOCKS5 connections",
		ConstLabels: prometheus.Labels{
			"network": c.network,
			"ip":      ip,
		},
	})

	socksGauge.Set(float64(value))

	pusher := push.New(c.pushGatewayURL, c.jobName).
		Grouping("node_id", c.nodeID).
		Collector(socksGauge)

	if err := pusher.Push(); err != nil {
		log.Errorf("metrics: push failed: online %v", err)
	}
}
