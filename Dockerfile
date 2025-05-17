FROM golang:1.24 AS builder

ARG NODE_TYPE=bootstrap

COPY . /warpnet
WORKDIR /warpnet

ENV GOPROXY=''
ENV GOSUMDB=''
ENV GOPRIVATE='github.com/Warp-net/warpnet'
ENV GO111MODULE=''
ENV CGO_ENABLED=0

RUN go build -ldflags "-s -w" -gcflags=all=-l -mod=vendor -v -o warpnet cmd/node/$NODE_TYPE/main.go

CMD ["/warpnet/warpnet"]
