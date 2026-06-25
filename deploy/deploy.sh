#!/bin/bash
set -e

# ── Firewall / NAT: идемпотентно и без разрушения сети хоста/чужих контейнеров ──
ufw disable 2>/dev/null || true

# НЕ перезапускаем systemd-networkd: это сбрасывает default route хоста и
# gateway-IP docker-мостов → все контейнеры падают в [Errno 101] Network is
# unreachable до следующей ручной починки. Именно это и роняло сервис.

WAN_IF="$(ip route show default | grep -oP 'dev \K\S+' | head -n1)"   # реальный аплинк, не eth0

ensure() {  # добавить правило, только если его ещё нет (re-deploy не плодит дубли)
  local t="$1" c="$2"; shift 2
  iptables -t "$t" -C "$c" "$@" 2>/dev/null || iptables -t "$t" -A "$c" "$@"
}

ensure filter INPUT -p tcp -m multiport --dports 4001:4099 -j ACCEPT
ensure filter INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT

# Egress контейнеров. Если dockerd сам управляет iptables (дефолт) — Docker уже
# делает это для всех сетей, и блок ниже не нужен; нужен только при iptables=false.
if iptables -L DOCKER-USER -n >/dev/null 2>&1; then
  ensure filter DOCKER-USER -j ACCEPT
fi
iptables -P FORWARD ACCEPT
if [ -n "$WAN_IF" ]; then
  ensure nat POSTROUTING -s 172.16.0.0/12 -o "$WAN_IF" -j MASQUERADE
fi

[ -L /etc/resolv.conf ] || printf 'nameserver 8.8.8.8\nnameserver 1.1.1.1\n' > /etc/resolv.conf
# ────────────────────────────────────────────────────────────────────────────

echo "Run deploy script"

if [ -z "$GITHUB_TOKEN" ]; then
  echo "Error: GITHUB_TOKEN is not set"
  exit 1
fi

echo $GITHUB_TOKEN | docker login ghcr.io -u filinvadim --password-stdin
docker pull ghcr.io/warp-net/warpnet-bootstrap:latest
docker pull ghcr.io/warp-net/warpnet-moderator:latest
docker pull ghcr.io/warp-net/warpnet-echo:latest
docker pull ghcr.io/warp-net/warpnet-vadim:latest

export HOSTNAME=''

if [ "$MAINNET" = "true" ]; then
    echo "Mainnet is enabled"
    mkdir -p /root/mainnet
    mv docker-compose-mainnet.yml mainnet/docker-compose-mainnet.yml
    docker compose -p warpnet-mainnet -f mainnet/docker-compose-mainnet.yml down --remove-orphans
    docker compose -p warpnet-mainnet -f mainnet/docker-compose-mainnet.yml up -d --build
else
    echo "Mainnet is disabled"
    mkdir -p /root/testnet
    mv docker-compose-testnet.yml testnet/docker-compose-testnet.yml
    docker compose -p warpnet-testnet -f testnet/docker-compose-testnet.yml down --remove-orphans
    docker compose -p warpnet-testnet -f testnet/docker-compose-testnet.yml up -d --build
fi
docker image prune --force
