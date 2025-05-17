all: deps
gx:
	go get github.com/whyrusleeping/gx
	go get github.com/whyrusleeping/gx-go

deps: gx 
	gx --verbose install --global
	gx-go rewrite


test: deps # just check that it builds as there is nothing to test
	go build .
publish:
	gx-go rewrite --undo
