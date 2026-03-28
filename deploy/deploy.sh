#!/bin/bash
set -e

# Firewall setup (errors are non-fatal — ufw may not be installed on all servers)
ufw disable 2>/dev/null || true
systemctl restart systemd-networkd || true

iptables -I DOCKER-USER -j ACCEPT 2>/dev/null || true
iptables -I INPUT -p tcp --match multiport --dports 4001:4099 -j ACCEPT
iptables -I INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
iptables -A FORWARD -i docker0 -o docker0 -j ACCEPT
iptables -P FORWARD ACCEPT
iptables -t nat -C POSTROUTING -o eth0 -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
echo "nameserver 8.8.8.8" > /etc/resolv.conf
echo "nameserver 1.1.1.1" >> /etc/resolv.conf

echo "Run deploy script"

echo "GITHUB_TOKEN: ${GITHUB_TOKEN:0:4}... (truncated for security)"

if [ -z "$GITHUB_TOKEN" ]; then
  echo "Error: GITHUB_TOKEN is not set"
  exit 1
fi

echo $GITHUB_TOKEN | docker login ghcr.io -u filinvadim --password-stdin
docker pull ghcr.io/warp-net/warpnet-bootstrap:latest
docker pull ghcr.io/warp-net/warpnet-moderator:latest

export HOSTNAME=''

if [ "$MAINNET" = "true" ]; then
    echo "Mainnet is enabled"
    mkdir -p /root/mainnet
    mv docker-compose-mainnet.yml mainnet/docker-compose-mainnet.yml
    docker compose -p warpnet-mainnet -f mainnet/docker-compose-mainnet.yml down --remove-orphans || true
    docker rm -f $(docker ps -aq --filter "name=warpnet-mainnet" --filter "name=push-gateway-mainnet") 2>/dev/null || true
    docker compose -p warpnet-mainnet -f mainnet/docker-compose-mainnet.yml up -d --force-recreate
else
    echo "Mainnet is disabled"
    mkdir -p /root/testnet
    mv docker-compose-testnet.yml testnet/docker-compose-testnet.yml
    docker compose -p warpnet-testnet -f testnet/docker-compose-testnet.yml down --remove-orphans || true
    docker rm -f $(docker ps -aq --filter "name=warpnet-testnet" --filter "name=push-gateway-testnet") 2>/dev/null || true
    docker compose -p warpnet-testnet -f testnet/docker-compose-testnet.yml up -d --force-recreate
fi
docker image prune --force