# go-libp2p-raft

[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](https://protocol.ai)
[![](https://img.shields.io/badge/project-libp2p-yellow.svg?style=flat-square)](https://libp2p.io)
[![](https://img.shields.io/badge/freenode-%23libp2p-yellow.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23libp2p)
[![GoDoc](https://godoc.org/github.com/libp2p/go-libp2p-raft?status.svg)](https://godoc.org/github.com/libp2p/go-libp2p-raft)
[![Build Status](https://travis-ci.com/libp2p/go-libp2p-raft.svg?branch=master)](https://travis-ci.com/libp2p/go-libp2p-raft)
[![codecov](https://codecov.io/gh/libp2p/go-libp2p-raft/branch/master/graph/badge.svg)](https://codecov.io/gh/libp2p/go-libp2p-raft)
[![Discourse posts](https://img.shields.io/discourse/https/discuss.libp2p.io/posts.svg)](https://discuss.libp2p.io)

> A LibP2P wrapper for hashicorp/raft implementation.

`go-libp2p-raft` implements a go-libp2p-consensus interface wrapping hashicorp/raftimplementation and providing a libp2p network transport for it.

## Table of Contents

- [Background](#background)
- [Install](#install)
- [Usage](#usage)
- [Contribute](#contribute)
- [License](#license)

## Background

[Raft is a fault-tolerant consensus algorithm](https://raft.github.io/) that allows a number of peers to agree on a State. Each peer can submit updates to that State, which are then distributed and agreed upon.

[Hashicorp's Raft](https://github.com/hashicorp/raft) is a mature implementation of the algorithm, which includes a number of performance improvements over the original version and a modular approach that allows to replace pieces of the system with custom implementations.

[LibP2P](https://github.com/libp2p) provides a modular peer-to-peer networking stack, which simplifies the networking layer by adding features like generic addressing, secure channels, protocol multiplexing over single connections or nat traversal in a standarized fashion.

`go-libp2p-raft` uses Hashicorp's Raft implementation and provides a LibP2P-based network transport for it. At the same time, it wraps the whole system with an implementation of the `go-libp2p-consensus` interface, ensuring any applications relying on it can easily swap it for a different one.

In short, `go-libp2p-raft`:

  * can be used to implement raft-consensus-based systems
  * takes advantange of the libp2p features (secure channels, protocol multiplexing, nat transversal...)
  * takes advantage of Hashicorp's Raft implementation matureness and performance improvementes (pipelining)
  * forms a layer which can easily be replaced by a different `go-libp2p-consensus` implementation.


## Install

`go-libp2p-raft` works like a regular Go library and uses Go modules for depdendency management:

## Usage

The documentation and examples for this module can be found in [godoc.org/github.com/libp2p/go-libp2p-raft](https://godoc.org/github.com/libp2p/go-libp2p-raft).

## Contribute

PRs accepted.

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

MIT © Protocol Labs, Inc.

---

The last gx published version of this module was: 1.2.20: QmX73JLtJ92tDcZajRrYtQDVSLQ5LPnADHwwQLXkTzNRhE
