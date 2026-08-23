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
)

type MetricsClient struct {
	pushGatewayURL string
	jobName        string
	network        string

	socksGauge  *prometheus.GaugeVec
	onlineGauge *prometheus.GaugeVec
	ratingGauge *prometheus.GaugeVec

	mx     sync.Mutex
	pusher *push.Pusher
}

func NewMetricsClient(pushGatewayURL, ownNodeId, network string) *MetricsClient {
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
		[]string{"peer_id"},
	)

	socksGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "socks_active_connections",
			Help: "Current number of active SOCKS5 connections",
		},
		[]string{"peer_id", "ip"},
	)
	// ratingGauge carries what peer rating WOULD have done while the
	// node runs in shadow mode. The offence weights are guesses that
	// cannot be calibrated once enforcement starts changing the
	// behaviour being measured, so shadow decisions have to be
	// aggregable across the fleet rather than sitting in one node's
	// log.
	ratingGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_rating_band",
			Help: "Observed peer rating band: 0 trusted, 1 watched, 2 degraded, 3 floor",
		},
		[]string{"peer_id", "band"},
	)

	pusher := push.New(pushGatewayURL, "warpnet_node").
		Grouping("network", network).
		Grouping("node_id", ownNodeId).
		Collector(onlineGauge).
		Collector(socksGauge).
		Collector(ratingGauge)

	return &MetricsClient{
		pushGatewayURL: pushGatewayURL,
		jobName:        "warpnet_node",
		onlineGauge:    onlineGauge,
		socksGauge:     socksGauge,
		ratingGauge:    ratingGauge,
		pusher:         pusher,
		network:        network,
	}
}

// PushRatingBand reports a peer's observed standing. Called only from
// shadow mode, where it is the whole point of the mode.
func (c *MetricsClient) PushRatingBand(peerId, band string) {
	if c == nil {
		return
	}
	c.mx.Lock()
	defer c.mx.Unlock()
	c.ratingGauge.WithLabelValues(peerId, band).Set(1)
	logPushResult(c.pusher.Push())
}

func (c *MetricsClient) PushStatusOnline(peerId string) {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.onlineGauge.WithLabelValues(peerId).Set(1)
	logPushResult(c.pusher.Push())
}

func (c *MetricsClient) PushStatusOffline(peerId string) {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.onlineGauge.WithLabelValues(peerId).Set(0)
	logPushResult(c.pusher.Push())
}

func (c *MetricsClient) PushSocksConnections(peerId, ip string) {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.socksGauge.WithLabelValues(peerId, ip).Set(1)
	logPushResult(c.pusher.Push())
}

func (c *MetricsClient) RemoveSocksConnections(peerId, ip string) {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.socksGauge.DeleteLabelValues(peerId, ip)
	logPushResult(c.pusher.Push())
}

func logPushResult(err error) {
	if err != nil {
		log.Errorf("metrics push result: %v", err)
	}
}
