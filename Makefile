kill:
	pkill -9 warpnet

backend-only-main:
	go run -tags backend cmd/node/member/backend-only.go --node.network testnet

backend-only-second:
	go run -tags backend cmd/node/member/backend-only.go --node.network testnet --node.port 4002 --node.seed backendtest --database.dir backend1

run-main:
	 cd cmd/node/member && wails build -m -nosyncgomod -devtools -tags webkit2_41 && ./build/bin/warpnet --node.network testnet

run-second:
	 cd cmd/node/member && wails build -m -nosyncgomod -devtools -tags webkit2_41 && ./build/bin/warpnet --node.network testnet --node.port 4002 --node.seed backendtest --database.dir backend1

run-moderator:
	CGO_CXXFLAGS="-w -Wno-format -Wno-delete-incomplete" go run -tags=llama cmd/node/moderator/main.go --node.network testnet --node.port 4002 --node.seed moderatorlocalhost --node.moderator.modelpath $(HOME)/.warpdata/llama-2-7b-chat.Q8_0.gguf 2>/dev/null

tests:
	CGO_ENABLED=0 go test -count=1 -short -v ./...

prune-testnet:
	rm -rf $(HOME)/.warpdata/testnet/*

check-heap:
	go build -gcflags="-m" main.go

update-deps:
	go get -v -u all && go mod vendor

setup-hooks:
	git config core.hooksPath .githooks

ssh-do:
	ssh root@207.154.221.44

snapcraft:
	sudo rm -rf parts/ stage/ prime/ overlay/ .craft/ *.snap
	sudo snapcraft pack --destructive-mode
	sudo snapcraft clean
	sudo rm -rf parts/ stage/ prime/ overlay/ .craft/

status:
	snapcraft status warpnet

build-windows:
	cd cmd/node/member && wails build -clean -platform windows -tags webkit2_41 -m -nosyncgomod --node.network testnet && cd -

download-golang-armv6:
	wget -c https://go.dev/dl/go1.25.3.linux-armv6l.tar.gz