#!/usr/bin/env bash
#
# Launch a group of Warpnet echo nodes on one host for load testing.
#
#   ./run-nodes.sh build
#   BOOTSTRAP=/ip4/192.168.1.10/tcp/4100/p2p/12D3... ./run-nodes.sh start
#   ./run-nodes.sh status
#   ./run-nodes.sh targets            # Prometheus file_sd JSON for the collector host
#   ./run-nodes.sh stop
#
# Second host differs only in INDEX_BASE / PORT_BASE / METRICS_PORT_BASE so that
# node indices, ports and seeds stay globally unique across both laptops.
#
#   host A:  INDEX_BASE=0  PORT_BASE=4100 METRICS_PORT_BASE=9100 COUNT=50
#   host B:  INDEX_BASE=50 PORT_BASE=4200 METRICS_PORT_BASE=9200 COUNT=50

set -euo pipefail

COUNT=${COUNT:-50}
INDEX_BASE=${INDEX_BASE:-0}
PORT_BASE=${PORT_BASE:-4100}
METRICS_PORT_BASE=${METRICS_PORT_BASE:-9100}

NETWORK=${NETWORK:-loadtest}
BOOTSTRAP=${BOOTSTRAP:-}
BIND_V4=${BIND_V4:-0.0.0.0}
LAN_IP=${LAN_IP:-}

# Stock deploy profile (deploy/docker-compose-testnet.yml). Lower these only
# together with the badger low-memory profile, and never silently.
GOMEMLIMIT_PER_NODE=${GOMEMLIMIT_PER_NODE:-288MiB}
GOMAXPROCS_PER_NODE=${GOMAXPROCS_PER_NODE:-2}

# A libp2p node in a 100-peer mesh holds far more sockets than the macOS
# per-process default of 256.
FD_LIMIT=${FD_LIMIT:-4096}

# Starting 50 nodes at once saturates the discovery leaky bucket
# (core/discovery/discovery.go: capacity 32, 2 leaks per 10s) and every node
# spends the first minutes rate-limited instead of connecting.
STAGGER_MS=${STAGGER_MS:-400}

LOG_LEVEL=${LOG_LEVEL:-info}
LOG_FORMAT=${LOG_FORMAT:-json}

RUN_DIR=${RUN_DIR:-"$HOME/.warpnet-loadtest"}
REPO=${REPO:-"$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"}
BIN=${BIN:-"$RUN_DIR/bin/warpnet-echo"}

PID_DIR="$RUN_DIR/pids"
LOG_DIR="$RUN_DIR/logs"
DATA_DIR="$RUN_DIR/data"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

node_index()  { echo $(( INDEX_BASE + $1 )); }
node_port()   { echo $(( PORT_BASE + $1 )); }
node_mport()  { echo $(( METRICS_PORT_BASE + $1 )); }
node_name()   { echo "echo-$(node_index "$1")"; }

detect_lan_ip() {
  [ -n "$LAN_IP" ] && { echo "$LAN_IP"; return; }
  local iface addr
  if command -v ipconfig >/dev/null 2>&1; then
    iface=$(route -n get default 2>/dev/null | awk '/interface:/{print $2}') || true
    [ -n "$iface" ] && ipconfig getifaddr "$iface" 2>/dev/null && return
    for i in en0 en1; do
      ipconfig getifaddr "$i" 2>/dev/null && return
    done
  elif command -v ip >/dev/null 2>&1; then
    addr=$(ip -4 route get 1.1.1.1 2>/dev/null \
      | awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit }}') || true
    [ -n "$addr" ] && { echo "$addr"; return; }
  fi
  die "could not detect the LAN address; set LAN_IP=<addr>"
}

# macOS keeps the machine awake with caffeinate, systemd with an inhibitor lock.
# Neither is fatal: a desktop that never sleeps simply has nothing to inhibit.
inhibit_sleep() {
  local pidfile="$RUN_DIR/caffeinate.pid"
  [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null && return 0
  # The inhibitor outlives this script, so it must not keep the script's stdout
  # open: a caller that pipes us (./run-nodes.sh start | tail) would hang on it.
  if command -v caffeinate >/dev/null 2>&1; then
    caffeinate -dimsu </dev/null >/dev/null 2>&1 & echo $! > "$pidfile"
  elif command -v systemd-inhibit >/dev/null 2>&1; then
    systemd-inhibit --what=idle:sleep --who=warpnet-loadtest \
      --why="load test in progress" sleep infinity </dev/null >/dev/null 2>&1 & echo $! > "$pidfile"
  else
    info "no sleep inhibitor available — keep the machine awake yourself"
    return 0
  fi
  info "sleep inhibited (pid $(cat "$pidfile"))"
}

cmd_build() {
  mkdir -p "$(dirname "$BIN")"
  info "building echo node from $REPO"
  ( cd "$REPO" && CGO_ENABLED=0 go build -tags echo -mod=vendor \
      -o "$BIN" ./cmd/node/member/echo-member.go )
  info "built $BIN"
}

cmd_start() {
  [ -x "$BIN" ] || die "no binary at $BIN — run '$0 build' first"
  [ -n "$BOOTSTRAP" ] || die "BOOTSTRAP is required: the '$NETWORK' network has no built-in
bootstrap list (config/config.go only seeds it for 'warpnet' and 'testnet'), so
pass the relay multiaddrs of BOTH hosts, comma separated"

  mkdir -p "$PID_DIR" "$LOG_DIR" "$DATA_DIR"
  ulimit -n "$FD_LIMIT" || die "could not raise the fd limit to $FD_LIMIT"

  # A laptop that falls asleep mid-run takes its half of the network with it.
  inhibit_sleep

  local lan; lan=$(detect_lan_ip)
  info "starting $COUNT nodes on $lan, indices $(node_index 0)..$(node_index $((COUNT - 1)))"

  local stagger_s; stagger_s=$(awk "BEGIN{print $STAGGER_MS/1000}")
  local i name port mport pidfile
  for (( i = 0; i < COUNT; i++ )); do
    name=$(node_name "$i")
    port=$(node_port "$i")
    mport=$(node_mport "$i")
    pidfile="$PID_DIR/$name.pid"

    if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
      info "  $name already running (pid $(cat "$pidfile"))"
      continue
    fi

    # NODE_METRICS_* and NODE_MDNS are passed already but stay inert until phase
    # 0 tasks 0.6 and 0.4 land. Everything above them is read by viper today
    # (config.go: AutomaticEnv, "." -> "_").
    env \
      NODE_NETWORK="$NETWORK" \
      NODE_PORT="$port" \
      NODE_HOST_V4="$BIND_V4" \
      NODE_SEED="$name" \
      NODE_BOOTSTRAP="$BOOTSTRAP" \
      DATABASE_DIR="$name" \
      LOGGING_LEVEL="$LOG_LEVEL" \
      LOGGING_FORMAT="$LOG_FORMAT" \
      GOMEMLIMIT="$GOMEMLIMIT_PER_NODE" \
      GOMAXPROCS="$GOMAXPROCS_PER_NODE" \
      ECHO_INDEX="$(node_index "$i")" \
      NODE_METRICS_HOST="$lan" \
      NODE_METRICS_PORT="$mport" \
      NODE_MDNS="${NODE_MDNS:-on}" \
      WARP_LOADTEST="${WARP_LOADTEST:-0}" \
      ECHO_TWEET_INTERVAL="${ECHO_TWEET_INTERVAL:-24h}" \
      ECHO_REACT_PERCENT="${ECHO_REACT_PERCENT:-100}" \
      ECHO_RETWEET_PERCENT="${ECHO_RETWEET_PERCENT:-25}" \
      ECHO_REPLY_PERCENT="${ECHO_REPLY_PERCENT:-25}" \
      ECHO_FOLLOW_COUNT="${ECHO_FOLLOW_COUNT:-5}" \
      ECHO_FOLLOW_DELAY="${ECHO_FOLLOW_DELAY:-90s}" \
      ECHO_REACT_INTERVAL="${ECHO_REACT_INTERVAL:-30s}" \
      ECHO_REACT_BATCH="${ECHO_REACT_BATCH:-2000}" \
      "$BIN" >"$LOG_DIR/$name.log" 2>&1 &

    echo $! > "$pidfile"
    sleep "$stagger_s"
  done

  info "started. logs in $LOG_DIR, pids in $PID_DIR"
  info "next: '$0 targets' and hand the file to the collector host"
}

cmd_stop() {
  local pidfile pid pids=()

  # echo-member.go only registers os.Interrupt/SIGINT — SIGTERM is not handled,
  # so a TERM would kill it before it closes the node.
  for pidfile in "$PID_DIR"/echo-*.pid; do
    [ -e "$pidfile" ] || continue
    pid=$(cat "$pidfile")
    if kill -0 "$pid" 2>/dev/null; then
      kill -INT "$pid" 2>/dev/null || true
      pids+=("$pid")
    fi
    rm -f "$pidfile"
  done

  if [ "${#pids[@]}" -eq 0 ]; then
    info "no running nodes found"
  else
    info "SIGINT sent to ${#pids[@]} nodes; waiting up to 20s for clean shutdown"
    local waited=0 alive
    while [ "$waited" -lt 20 ]; do
      alive=0
      for pid in "${pids[@]}"; do
        kill -0 "$pid" 2>/dev/null && alive=$((alive + 1))
      done
      [ "$alive" -eq 0 ] && break
      sleep 1; waited=$((waited + 1))
    done
    for pid in "${pids[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        info "force-killing $pid"
        kill -KILL "$pid" 2>/dev/null || true
      fi
    done
  fi

  if [ -f "$RUN_DIR/caffeinate.pid" ]; then
    kill "$(cat "$RUN_DIR/caffeinate.pid")" 2>/dev/null || true
    rm -f "$RUN_DIR/caffeinate.pid"
    info "sleep inhibitor released"
  fi
}

cmd_status() {
  local i name pidfile pid alive=0 dead=0
  for (( i = 0; i < COUNT; i++ )); do
    name=$(node_name "$i")
    pidfile="$PID_DIR/$name.pid"
    if [ -f "$pidfile" ] && pid=$(cat "$pidfile") && kill -0 "$pid" 2>/dev/null; then
      alive=$((alive + 1))
    else
      dead=$((dead + 1))
      [ -f "$pidfile" ] && info "  down: $name (last log: $(tail -n1 "$LOG_DIR/$name.log" 2>/dev/null | cut -c1-120))"
    fi
  done
  info "alive $alive / down $dead / expected $COUNT"

  if [ "$alive" -gt 0 ]; then
    local rss
    rss=$(ps -o rss= -p "$(cat "$PID_DIR"/echo-*.pid 2>/dev/null | tr '\n' ',' | sed 's/,$//')" 2>/dev/null \
      | awk '{s+=$1; n++} END{if(n) printf "%.0f MiB total, %.0f MiB/node over %d", s/1024, s/1024/n, n}')
    [ -n "$rss" ] && info "rss: $rss"
  fi
}

cmd_targets() {
  local lan i out
  mkdir -p "$RUN_DIR"
  lan=$(detect_lan_ip)
  out="$RUN_DIR/prometheus-targets-$lan.json"
  {
    printf '[{"labels":{"job":"warpnet-echo","host":"%s"},"targets":[' "$lan"
    for (( i = 0; i < COUNT; i++ )); do
      [ "$i" -gt 0 ] && printf ','
      printf '"%s:%s"' "$lan" "$(node_mport "$i")"
    done
    printf ']}]\n'
  } > "$out"
  info "$out"
}

case "${1:-}" in
  build)   cmd_build ;;
  start)   cmd_start ;;
  stop)    cmd_stop ;;
  status)  cmd_status ;;
  targets) cmd_targets ;;
  *) die "usage: $0 {build|start|stop|status|targets}" ;;
esac
