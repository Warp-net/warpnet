kill:
	pkill -9 warpnet

dry-run-main:
	go run -tags dryrun cmd/node/member/dry-run.go --node.network testnet

dry-run-second:
	go run -tags dryrun cmd/node/member/dry-run.go --node.network testnet --node.port 4002 --node.seed dryruntest --database.dir dryrun1

run-member:
	 cd cmd/node/member && wails build -m -nosyncgomod -devtools -tags webkit2_41 && ./build/bin/warpnet --node.network testnet

moderator:
	go run -tags=llama cmd/node/moderator/main.go --node.network testnet

tests:
	CGO_ENABLED=0 go test -count=1 -short -v ./...

prune-testnet:
	(pkill -9 main || true) && rm -rf $(HOME)/.warpdata/testnet/*

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

