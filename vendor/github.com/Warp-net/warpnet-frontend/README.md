# WARPNET-FRONTEND

[MAIN REPO](https://github.com/Warp-net/warpnet)
[DOCS](https://github.com/Warp-net/docs)

## Requirements
### Development
* npm version >= 9.2.0
### Production
* npm version >= 9.2.0
* [Golang v1.24](https://go.dev/doc/install)
* [Wails v2.10.2](https://github.com/wailsapp/wails)

## How to run node (dev mode)

```bash 
  npm run serve
```
## How to run node (prod mode)
* commit and push your frontend changes **(INCLUDING DIST FOLDER!)**
* switch to main repo `github.com/Warp-net/warpnet` and call command:
```bash 
  go get github.com/Warp-net/warpnet-frontend@latest && go mod vendor
```
* then in the same main repo run node:
```bash
  make run-member
```
