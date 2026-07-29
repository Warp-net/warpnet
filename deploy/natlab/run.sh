#!/usr/bin/env bash
#
# Runs the NAT hole-punch harness. No sudo needed: membership in the docker
# group is enough, and --privileged gives the container CAP_NET_ADMIN inside
# its own network namespace. --network none keeps the lab off every real
# network, so nothing can reach mainnet, testnet or the host LAN.
#
#   ./deploy/natlab/run.sh                 # build + run
#   FORCE_PRIVATE=1 ./deploy/natlab/run.sh # skip AutoNAT reachability probing
#   ./deploy/natlab/run.sh --shell         # interactive shell in the lab

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
image=${IMAGE:-warpnet-natlab:latest}

cd "$repo_root"

echo "== building $image"
docker build -f deploy/natlab/Dockerfile -t "$image" .

run_args=(
  --rm
  --privileged
  --network none
  -e "FORCE_PRIVATE=${FORCE_PRIVATE:-0}"
  -e "PEER_TIMEOUT=${PEER_TIMEOUT:-180s}"
)

if [ "${1:-}" = "--shell" ]; then
  echo "== opening a shell in the lab (run /usr/local/bin/topology.sh by hand)"
  exec docker run -it "${run_args[@]}" --entrypoint /bin/bash "$image"
fi

echo "== running the lab"
exec docker run "${run_args[@]}" "$image"
