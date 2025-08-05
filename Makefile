kill:
	pkill -9 main

dry-run:
	go run -tags dryrun cmd/node/member/dry-run.go --node.network testnet

run-member:
	 cd cmd/node/member && wails build -devtools -tags webkit2_41 && ./build/bin/warpnet --node.network testnet

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
