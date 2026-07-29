FROM golang:1.26

RUN apt-get update && apt-get install -y --no-install-recommends \
      iproute2 iptables iputils-ping \
    && rm -rf /var/lib/apt/lists/*

COPY . /warpnet
WORKDIR /warpnet

ENV CGO_ENABLED=0

RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -mod=vendor -o /usr/local/bin/natlab ./deploy/natlab

RUN cp /warpnet/deploy/natlab/topology.sh /usr/local/bin/topology.sh \
    && chmod +x /usr/local/bin/topology.sh

ENTRYPOINT ["/usr/local/bin/topology.sh"]
