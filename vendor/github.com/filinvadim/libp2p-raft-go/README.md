# libp2p-raft-go

[![](https://img.shields.io/badge/project-libp2p-yellow.svg?style=flat-square)](https://libp2p.io)
[![](https://img.shields.io/badge/freenode-%23libp2p-yellow.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23libp2p)
[![Discourse posts](https://img.shields.io/discourse/https/discuss.libp2p.io/posts.svg)](https://discuss.libp2p.io)

> A LibP2P wrapper for hashicorp/raft implementation.

Inspired but unsatisfied with [go-libp2p-raft](https://github.com/libp2p/go-libp2p-raft) which doesn't help to make
Raft integration easier.

`libp2p-raft-go` implements a  libp2p Raft consensus interface wrapping hashicorp/raft implementation and 
providing a libp2p network transport for it.

## Table of Contents

- [Background](#background)
- [Install](#install)
- [Usage](#usage)
- [Tests](#tests)
- [Contribute](#contribute)
- [License](#license)

## Background

[Raft is a fault-tolerant consensus algorithm](https://raft.github.io/) that allows a number of peers
to agree on a State. Each peer can submit updates to that State, which are then distributed and 
agreed upon.

[Hashicorp's Raft](https://github.com/hashicorp/raft) is a mature implementation of the algorithm,
which includes a number of performance improvements over the original version and a modular approach
that allows to replace pieces of the system with custom implementations.

[LibP2P](https://github.com/libp2p) provides a modular peer-to-peer networking stack, which simplifies
the networking layer by adding features like generic addressing, secure channels, protocol 
multiplexing over single connections or nat traversal in a standarized fashion.

`libp2p-raft-go` uses Hashicorp's Raft implementation and provides a LibP2P-based network transport
for it. At the same time, it wraps the whole system with an implementation of the libp2p Raft consensus
interface, ensuring any applications relying on it can easily swap it for a different one.

In short, `libp2p-raft-go`:

* can be used to implement raft-consensus-based systems
* takes advantange of the libp2p features (secure channels, protocol multiplexing, nat transversal...)
* takes advantage of Hashicorp's Raft implementation matureness and performance improvementes (pipelining)
* forms a layer which can easily be replaced by a different libp2p Raft consensus implementation.


## Install

`libp2p-raft-go` works like a regular Go library and uses Go modules for depedendency management:

## Usage

https://github.com/filinvadim/libp2p-raft-go/blob/master/example/main.go#L86-L217

## Tests

TODO.
Tested in [WarpNet](https://github.com/Warp-net/warpnet) production environment.

## Contribute

Contributions are what make the open source community such an amazing place to learn, inspire, and create.
Any contributions you make are **greatly appreciated**.

If you have a suggestion that would make this better, please fork the repo and create a pull request.
You can also open an issue with the tag "enhancement."
Remember to give the project a star! Thanks again!

1. Fork the Project
2. Create your Feature Branch (`git checkout -b IssueNumber/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin IssueNumber/AmazingFeature`)
5. Open a Pull Request

## License

MIT © Vadim Filin

---
