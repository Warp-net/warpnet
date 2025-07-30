kill:
	pkill -9 main

run-member:
	 cd cmd/node/member && wails build -tags webkit2_41 && ./build/bin/warpnet --node.network testnet

second-member:
	go run cmd/node/member/main.go --database.dir storage2 --node.port 4021 --server.port 4022 --node.network testnet

third-member:
	go run cmd/node/member/main.go --database.dir storage3 --node.port 4031 --server.port 4032  --node.network testnet

moderator:
	go run -tags=llama cmd/node/moderator/main.go --node.network testnet

tests:
	CGO_ENABLED=0 go test -count=1 -short -v ./...

prune:
	(pkill -9 main || true) && rm -rf $(HOME)/.warpdata

check-heap:
	go build -gcflags="-m" main.go

update-deps:
	go get -v -u all && go mod vendor

setup-hooks:
	git config core.hooksPath .githooks

ssh-do:
	ssh root@207.154.221.44

build-macos:
	GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -gcflags=all=-l -mod=vendor -v -o warpnet-darwin cmd/node/member/main.go
	chmod +x warpnet-darwin

gui:
	go run -tags="wails production desktop webkit2_41" cmd/wails/main.go
