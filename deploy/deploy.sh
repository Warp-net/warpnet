#!/bin/bash

ufw disable
systemctl restart systemd-networkd
iptables -I DOCKER-USER -j ACCEPT
iptables -A INPUT -p tcp --match multiport --dports 4001:4099 -j ACCEPT
iptables -A FORWARD -i docker0 -o docker0 -j ACCEPT
iptables -A FORWARD -i br-6383b19e4979 -o br-6383b19e4979 -j ACCEPT
iptables -P FORWARD ACCEPT
iptables -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
echo "nameserver 8.8.8.8" > /etc/resolv.conf
echo "nameserver 1.1.1.1" >> /etc/resolv.conf

echo "Run deploy script"

echo "GITHUB_TOKEN: ${GITHUB_TOKEN:0:4}... (truncated for security)"

if [ -z "$GITHUB_TOKEN" ]; then
  echo "Error: GITHUB_TOKEN is not set"
  exit 1
fi

echo $GITHUB_TOKEN | sudo docker login ghcr.io -u filinvadim --password-stdin
docker pull ghcr.io/warp-net/warpnet-bootstrap:latest
#docker pull ghcr.io/warp-net/warpnet-moderator:latest

export HOSTNAME=''

if [ "$MAINNET" = "true" ]; then
    echo "Mainnet is enabled"
    docker compose -p warpnet-mainnet -f mainnet/docker-compose-mainnet.yml down --remove-orphans
    docker compose -p warpnet-mainnet -f mainnet/docker-compose-mainnet.yml up -d --build
else
    echo "Mainnet is disabled"
    docker compose -p warpnet-testnet -f testnet/docker-compose-testnet.yml down --remove-orphans
    docker compose -p warpnet-testnet -f testnet/docker-compose-testnet.yml up -d --build
fi
docker image prune --force