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

package metrics

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	log "github.com/sirupsen/logrus"
)

const namespace = "warpnet"

// Counters for the losses the node currently takes in silence. Both subsystems
// drop on back pressure without a word, so a delivery shortfall cannot be
// attributed to anything until these have numbers.
var (
	PubSubUndeliverable = counter("pubsub_undeliverable_total",
		"Messages dropped because a subscription's consumer did not read fast enough.")
	PubSubDropRPC = counter("pubsub_droprpc_total",
		"Outbound RPCs dropped, typically because the peer's queue was full.")
	PubSubDuplicate = counter("pubsub_duplicate_total",
		"Duplicate messages dropped.")
	PubSubThrottled = counter("pubsub_throttled_total",
		"Peers throttled by the peer gater.")
	PubSubDelivered = counter("pubsub_delivered_total",
		"Messages delivered to subscribers.")

	CRDTDeltasReceived = counter("crdt_deltas_received_total",
		"CRDT deltas handed to the broadcaster.")
	CRDTDeltasDropped = counter("crdt_deltas_dropped_total",
		"CRDT deltas discarded because the broadcaster queue was full.")
	CRDTQueueDepth = gauge("crdt_queue_depth",
		"CRDT deltas waiting in the broadcaster queue.")
)

func counter(name, help string) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace, Name: name, Help: help,
	})
	prometheus.DefaultRegisterer.MustRegister(c)
	return c
}

func gauge(name, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace, Name: name, Help: help,
	})
	prometheus.DefaultRegisterer.MustRegister(g)
	return g
}

// Serve exposes the default gatherer on host:port in Prometheus text format and
// returns once the listener is bound. promhttp is not vendored and is not needed
// here: encoding a gather by hand is the whole handler. libp2p reports onto the
// same default registry, so whatever it exports is served alongside.
func Serve(ctx context.Context, host, port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", handle)

	listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Errorf("metrics: server stopped: %v", err)
		}
	}()

	log.Infof("metrics: serving /metrics on %s", listener.Addr().String())
	return nil
}

func handle(w http.ResponseWriter, r *http.Request) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil && len(families) == 0 {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	format := expfmt.Negotiate(r.Header)
	w.Header().Set("Content-Type", string(format))

	encoder := expfmt.NewEncoder(w, format)
	for _, family := range families {
		if err := encoder.Encode(family); err != nil {
			log.Warnf("metrics: encode %s: %v", family.GetName(), err)
			return
		}
	}
}
