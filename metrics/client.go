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
	pushGatewayURL string
	jobName        string
	nodeID         string
	statusOnce     sync.Once
	stopChan       chan struct{}
}

func NewMetricsClient(pushGatewayURL string, nodeID string) *MetricsClient {
	return &MetricsClient{
		pushGatewayURL: pushGatewayURL,
		jobName:        "warpnet_node",
		nodeID:         nodeID,
		statusOnce:     sync.Once{},
		stopChan:       make(chan struct{}),
	}
}

func (client *MetricsClient) Stop() {
	close(client.stopChan)
}

func (client *MetricsClient) PushStatusOnline(network string, nodeType string) {
	if client.pushGatewayURL == "" {
		return
	}
	if network == "" {
		log.Errorf("metrics: network is empty")
		return
	}

	client.statusOnce.Do(func() {
		go func() {
			client.pushStatusOnline(network, nodeType)

			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-client.stopChan:
					return
				case <-ticker.C:
					client.pushStatusOnline(network, nodeType)
				}
			}
		}()
	})
}

func (client *MetricsClient) pushStatusOnline(network string, nodeType string) {
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
		log.Errorf("metrics: push failed: online %v", err)
	}
}

func (client *MetricsClient) PushSocksConnNumAdd(network string) {
	client.pushSocksConnNum(network, 1)
}

func (client *MetricsClient) PushSocksConnNumRemove(network string) {
	client.pushSocksConnNum(network, 0)
}

func (client *MetricsClient) pushSocksConnNum(network string, num int) {
	socksGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "node_socks_conn_num",
		Help: "SOCKS5 connection num",
		ConstLabels: prometheus.Labels{
			"network": network,
		},
	})
	socksGauge.Set(float64(num))

	pusher := push.New(client.pushGatewayURL, client.jobName).
		Grouping("node_id", client.nodeID).
		Collector(socksGauge)

	if err := pusher.Push(); err != nil {
		log.Errorf("metrics: push failed: %v", err)
	}
}

func (client *MetricsClient) PushSocksConnections(network string, value int64) {
	socksGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "socks_active_connections",
		Help: "Current number of active SOCKS5 connections",
		ConstLabels: prometheus.Labels{
			"network": network,
		},
	})

	socksGauge.Set(float64(value))

	pusher := push.New(client.pushGatewayURL, client.jobName).
		Grouping("node_id", client.nodeID).
		Collector(socksGauge)

	if err := pusher.Push(); err != nil {
		log.Errorf("metrics: push failed: %s %v", "socks5", err)
	}
}
