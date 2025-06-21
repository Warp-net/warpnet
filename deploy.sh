#!/bin/bash

echo "Run deploy script"
#sudo echo "nameserver 8.8.8.8" > /etc/resolv.conf
#sudo echo "nameserver 1.1.1.1" >> /etc/resolv.conf
echo "GITHUB_TOKEN: ${GITHUB_TOKEN:0:4}... (truncated for security)"
echo "NODE_HOST: ${NODE_HOST}"
if [ -z "$GITHUB_TOKEN" ]; then
  echo "Error: GITHUB_TOKEN is not set"
  exit 1
fi
if [ -z "${NODE_HOST}" ]; then
  echo "Error: NODE_HOST is not set"
fi
#sudo docker stop $(sudo docker ps -aq) || true
#sudo docker container prune -f
sudo docker compose -f docker-compose-warpnet.yml down || true
echo $GITHUB_TOKEN | sudo docker login ghcr.io -u filinvadim --password-stdin
sudo docker pull ghcr.io/warp-net/warpnet-bootstrap:latest
#sudo ufw disable
#sudo systemctl restart systemd-networkd
#sudo iptables -I DOCKER-USER -j ACCEPT
#sudo iptables -A INPUT -p tcp --match multiport --dports 4001:4003 -j ACCEPT
#sudo iptables -A FORWARD -i docker0 -o docker0 -j ACCEPT
#sudo iptables -A FORWARD -i br-6383b19e4979 -o br-6383b19e4979 -j ACCEPT
#sudo iptables -P FORWARD ACCEPT
#sudo iptables -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
#sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
sudo touch /tmp/snapshot || true
export HOSTNAME=''
export NODE_HOST=$NODE_HOST
sudo -E docker compose -f docker-compose-warpnet.yml up -d --build
sudo docker image prune -f
