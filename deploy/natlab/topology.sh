#!/usr/bin/env bash
#
# Builds a real NAT topology inside the container's own network namespace and
# runs a Warpnet hole punch across it. Requires CAP_NET_ADMIN (docker
# --privileged); nothing outside the container is touched.
#
#   root ns of the container = "the internet"
#     bridge inet 11.0.0.0/24
#     ├── relay          11.0.0.1     public: circuit relay + AutoNAT server
#     ├── [ns rtr-a] wan 11.0.0.11    MASQUERADE, lan 10.1.0.1/24
#     │        └── [ns host-a] 10.1.0.2  peer A (dials over the circuit)
#     └── [ns rtr-b] wan 11.0.0.12    MASQUERADE, lan 10.2.0.1/24
#              └── [ns host-b] 10.2.0.2  peer B (accepts, drives DCUtR)
#
# 11.0.0.0/24 is used for the fake internet on purpose: go-multiaddr counts
# 100.64.0.0/10 as private (RFC 6598), so a CGNAT range would make libp2p
# discard the observed addresses and no hole punch candidate would exist.

set -euo pipefail

NATLAB_BIN=${NATLAB_BIN:-/usr/local/bin/natlab}
LOG_DIR=${LOG_DIR:-/var/log/natlab}
RUN_DIR=${RUN_DIR:-/run/natlab}
PORT=${PORT:-4001}
PEER_TIMEOUT=${PEER_TIMEOUT:-180s}
FORCE_PRIVATE=${FORCE_PRIVATE:-0}

RELAY_IP=11.0.0.1
WAN_A=11.0.0.11
WAN_B=11.0.0.12
LAN_A_GW=10.1.0.1
LAN_A_HOST=10.1.0.2
LAN_B_GW=10.2.0.1
LAN_B_HOST=10.2.0.2

RELAY_SEED=natlab-relay
SEED_A=natlab-peer-a
SEED_B=natlab-peer-b

pids=()

say() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
die() { printf '\033[31mFAIL: %s\033[0m\n' "$*" >&2; exit 1; }

cleanup() {
  local pid
  for pid in "${pids[@]:-}"; do
    [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
  done
  for ns in host-a host-b rtr-a rtr-b; do
    ip netns del "$ns" 2>/dev/null || true
  done
  ip link del inet 2>/dev/null || true
}
trap cleanup EXIT

# ---------------------------------------------------------------- topology

build_router() {
  local rtr=$1 host=$2 wan_if=$3 wan_ip=$4 lan_if=$5 lan_gw=$6 host_if=$7 host_ip=$8 lan_cidr=$9

  ip netns add "$rtr"
  ip netns add "$host"

  # WAN leg: router <-> the fake internet bridge.
  ip link add "$wan_if" type veth peer name "br-$wan_if"
  ip link set "br-$wan_if" master inet up
  ip link set "$wan_if" netns "$rtr"
  ip -n "$rtr" addr add "$wan_ip/24" dev "$wan_if"
  ip -n "$rtr" link set "$wan_if" up

  # LAN leg: router <-> the NATed host.
  ip link add "$lan_if" type veth peer name "$host_if"
  ip link set "$lan_if" netns "$rtr"
  ip link set "$host_if" netns "$host"
  ip -n "$rtr" addr add "$lan_gw/24" dev "$lan_if"
  ip -n "$rtr" link set "$lan_if" up
  ip -n "$rtr" link set lo up
  ip -n "$host" addr add "$host_ip/24" dev "$host_if"
  ip -n "$host" link set "$host_if" up
  ip -n "$host" link set lo up
  ip -n "$host" route add default via "$lan_gw"

  ip netns exec "$rtr" sh -c 'echo 1 > /proc/sys/net/ipv4/ip_forward'

  # Port-preserving source NAT and no inbound port forwarding at all:
  # endpoint-independent mapping with address-dependent filtering, the NAT
  # class DCUtR is expected to traverse.
  ip netns exec "$rtr" iptables -t nat -A POSTROUTING -s "$lan_cidr" -o "$wan_if" -j MASQUERADE
  ip netns exec "$rtr" iptables -A FORWARD -i "$wan_if" -o "$lan_if" -m conntrack --ctstate NEW,INVALID -j DROP
  # Unsolicited inbound must be dropped silently, not rejected. An unmatched
  # SYN is destined to the router itself (INPUT, not FORWARD), and the default
  # RST would abort the peer's dial instead of letting TCP retransmit - which
  # is exactly what makes a simultaneous open succeed on a real NAT.
  ip netns exec "$rtr" iptables -A INPUT -i "$wan_if" -m conntrack --ctstate NEW,INVALID -j DROP
}

say "building topology"
ip link add inet type bridge
ip addr add "$RELAY_IP/24" dev inet
ip link set inet up

build_router rtr-a host-a wan-a "$WAN_A" lan-a "$LAN_A_GW" eth-a "$LAN_A_HOST" 10.1.0.0/24
build_router rtr-b host-b wan-b "$WAN_B" lan-b "$LAN_B_GW" eth-b "$LAN_B_HOST" 10.2.0.0/24

mkdir -p "$LOG_DIR" "$RUN_DIR"
rm -f "$RUN_DIR"/*.addr "$RUN_DIR"/*.done

ip -brief addr show
for ns in rtr-a rtr-b host-a host-b; do
  printf '  %-7s %s\n' "$ns" "$(ip -n "$ns" -brief addr show | tr '\n' ' ')"
done

say "sanity: NATed hosts can reach the relay"
ip netns exec host-a ping -c1 -W2 "$RELAY_IP" >/dev/null || die "host-a cannot reach the relay"
ip netns exec host-b ping -c1 -W2 "$RELAY_IP" >/dev/null || die "host-b cannot reach the relay"
echo "  ok: both LANs egress through their NAT"

# ------------------------------------------------------------------- nodes

# natlab takes no command-line flags: warpnet/config parses pflags from its
# init() and would reject them, so everything goes through the environment.
# The last line is the ID: warpnet prints a version banner to stdout on init.
peer_id() {
  NATLAB_ROLE=print-id NATLAB_SEED="$1" "$NATLAB_BIN" | tail -n1 | tr -d '[:space:]'
}

RELAY_ID=$(peer_id "$RELAY_SEED")
ID_A=$(peer_id "$SEED_A")
ID_B=$(peer_id "$SEED_B")
RELAY_MADDR="/ip4/$RELAY_IP/tcp/$PORT/p2p/$RELAY_ID"

say "peer identities"
printf '  relay  %s\n  peer-a %s\n  peer-b %s\n' "$RELAY_ID" "$ID_A" "$ID_B"

say "starting relay in the root namespace"
env NATLAB_ROLE=relay NATLAB_SEED="$RELAY_SEED" NATLAB_IP="$RELAY_IP" \
  NATLAB_PORT="$PORT" NATLAB_TIMEOUT="$PEER_TIMEOUT" \
  "$NATLAB_BIN" >"$LOG_DIR/relay.log" 2>&1 &
pids+=($!)

for _ in $(seq 40); do
  grep -q 'event=relay_up' "$LOG_DIR/relay.log" 2>/dev/null && break
  sleep 0.25
done
grep -q 'event=relay_up' "$LOG_DIR/relay.log" || { tail -30 "$LOG_DIR/relay.log"; die "relay did not start"; }
grep 'NATLAB' "$LOG_DIR/relay.log" | tail -2

say "starting peer B behind NAT B ($WAN_B)"
ip netns exec host-b env NATLAB_ROLE=peer NATLAB_SEED="$SEED_B" NATLAB_IP="$LAN_B_HOST" \
  NATLAB_PORT="$PORT" NATLAB_RELAY="$RELAY_MADDR" NATLAB_TARGET="$ID_A" \
  NATLAB_READY_FILE="$RUN_DIR/b.addr" NATLAB_DONE_FILE="$RUN_DIR/a.done" \
  NATLAB_TIMEOUT="$PEER_TIMEOUT" \
  NATLAB_FORCE_PRIVATE="$FORCE_PRIVATE" \
  "$NATLAB_BIN" >"$LOG_DIR/peer-b.log" 2>&1 &
pids+=($!)

say "waiting for peer B's relay reservation"
for _ in $(seq 240); do
  [ -s "$RUN_DIR/b.addr" ] && break
  grep -q 'RESULT=FAIL' "$LOG_DIR/peer-b.log" 2>/dev/null && break
  sleep 0.5
done
[ -s "$RUN_DIR/b.addr" ] || { tail -40 "$LOG_DIR/peer-b.log"; die "peer B never got a reservation"; }
echo "  circuit: $(cat "$RUN_DIR/b.addr")"

say "control: unsolicited inbound to NAT B must be blocked"
if ip netns exec host-a timeout 4 bash -c "exec 3<>/dev/tcp/$WAN_B/$PORT" 2>/dev/null; then
  die "direct dial to $WAN_B:$PORT succeeded - this is not a real NAT, the punch would be meaningless"
fi
echo "  ok: peer B is unreachable without hole punching, while it is listening"

say "starting peer A behind NAT A ($WAN_A) and dialing B over the relay"
ip netns exec host-a env NATLAB_ROLE=peer NATLAB_SEED="$SEED_A" NATLAB_IP="$LAN_A_HOST" \
  NATLAB_PORT="$PORT" NATLAB_RELAY="$RELAY_MADDR" NATLAB_TARGET="$ID_B" \
  NATLAB_DIAL_CIRCUIT=1 NATLAB_WAIT_FILE="$RUN_DIR/b.addr" \
  NATLAB_READY_FILE="$RUN_DIR/a.addr" NATLAB_DONE_FILE="$RUN_DIR/a.done" \
  NATLAB_TIMEOUT="$PEER_TIMEOUT" \
  NATLAB_FORCE_PRIVATE="$FORCE_PRIVATE" \
  "$NATLAB_BIN" >"$LOG_DIR/peer-a.log" 2>&1 &
pid_a=$!
pids+=("$pid_a")

say "waiting for the hole punch"
rc_a=0
wait "$pid_a" || rc_a=$?

# Peer B reports independently; give it a moment to finish its own assertions.
for _ in $(seq 40); do
  grep -qE 'RESULT=(PASS|FAIL)' "$LOG_DIR/peer-b.log" && break
  sleep 0.5
done

# ------------------------------------------------------------------ report

say "DCUtR trace"
grep -h 'event=dcutr' "$LOG_DIR/peer-a.log" "$LOG_DIR/peer-b.log" || echo "  (no DCUtR events)"

say "connections"
grep -h 'event=direct_conn' "$LOG_DIR/peer-a.log" "$LOG_DIR/peer-b.log" || echo "  (none)"
grep -h 'event=ping_ok' "$LOG_DIR/peer-a.log" "$LOG_DIR/peer-b.log" || true

say "NAT conntrack state on rtr-a"
if ip netns exec rtr-a test -r /proc/net/nf_conntrack; then
  ip netns exec rtr-a grep -E "dst=$WAN_B|src=$WAN_B" /proc/net/nf_conntrack || echo "  (no entry for $WAN_B)"
else
  echo "  (/proc/net/nf_conntrack unavailable on this kernel)"
fi

say "verdict"
fail=0
check() {
  if eval "$2" >/dev/null 2>&1; then
    printf '  \033[32mPASS\033[0m %s\n' "$1"
  else
    printf '  \033[31mFAIL\033[0m %s\n' "$1"
    fail=1
  fi
}

check "peer B reserved a relay slot"            "grep -q 'event=reservation' '$LOG_DIR/peer-b.log'"
check "peer A connected over the circuit"       "grep -q 'event=circuit_connected' '$LOG_DIR/peer-a.log'"
check "DCUtR reported a successful hole punch"  "grep -q 'type=EndHolePunch' '$LOG_DIR/peer-a.log' '$LOG_DIR/peer-b.log' && grep -q '\"Success\":true' '$LOG_DIR/peer-a.log' '$LOG_DIR/peer-b.log'"
check "peer A got a direct conn to $WAN_B"      "grep -q \"event=direct_conn.*remote='/ip4/$WAN_B\" '$LOG_DIR/peer-a.log'"
check "peer B got a direct conn to $WAN_A"      "grep -q \"event=direct_conn.*remote='/ip4/$WAN_A\" '$LOG_DIR/peer-b.log'"
check "peer A ping traversed the direct conn"   "grep -q 'event=ping_ok' '$LOG_DIR/peer-a.log'"
check "peer A exited clean"                     "[ $rc_a -eq 0 ]"
check "peer B reported PASS"                    "grep -q 'RESULT=PASS' '$LOG_DIR/peer-b.log'"

if [ "$fail" -ne 0 ]; then
  say "peer A log (tail)"; tail -40 "$LOG_DIR/peer-a.log"
  say "peer B log (tail)"; tail -40 "$LOG_DIR/peer-b.log"
  say "relay log (tail)";  tail -20 "$LOG_DIR/relay.log"
  die "hole punch harness did not pass"
fi

printf '\n\033[32mHOLE PUNCH VERIFIED: %s <-> %s across two independent NATs\033[0m\n' "$WAN_A" "$WAN_B"
