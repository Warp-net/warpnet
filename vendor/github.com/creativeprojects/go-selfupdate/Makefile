# 
# Makefile for go-selfupdate
# 
GOCMD=go
GOBUILD=$(GOCMD) build
GOINSTALL=$(GOCMD) install
GORUN=$(GOCMD) run
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOTOOL=$(GOCMD) tool
GOGET=$(GOCMD) get
GOPATH?=`$(GOCMD) env GOPATH`

TESTS=. ./update
COVERAGE_FILE=coverage.txt

BUILD_DATE=`date`
BUILD_COMMIT=`git rev-parse HEAD`

README=README.md
TOC_START=<\!--ts-->
TOC_END=<\!--te-->
TOC_PATH=toc.md

.PHONY: all test build coverage full-coverage clean toc

all: test build

build:
		$(GOBUILD) -v ./...

test:
		$(GOTEST) -race -v $(TESTS)

coverage:
		$(GOTEST) -short -coverprofile=$(COVERAGE_FILE) $(TESTS)
		$(GOTOOL) cover -html=$(COVERAGE_FILE)

full-coverage:
		$(GOTEST) -coverprofile=$(COVERAGE_FILE) $(TESTS)
		$(GOTOOL) cover -html=$(COVERAGE_FILE)

clean:
		rm detect-latest-release go-get-release coverage.txt
		$(GOCLEAN)

toc:
	@echo "[*] $@"
	$(GOINSTALL) github.com/ekalinin/github-markdown-toc.go/cmd/gh-md-toc@latest
	cat ${README} | gh-md-toc --hide-footer > ${TOC_PATH}
	sed -i ".1" "/${TOC_START}/,/${TOC_END}/{//!d;}" "${README}"
	sed -i ".2" "/${TOC_START}/r ${TOC_PATH}" "${README}"
	rm ${README}.1 ${README}.2 ${TOC_PATH}

.PHONY: lint
lint:
	@echo "[*] $@"
	GOOS=darwin golangci-lint run
	GOOS=linux golangci-lint run
	GOOS=windows golangci-lint run

.PHONY: fix
fix:
	@echo "[*] $@"
	$(GOCMD) mod tidy
	$(GOCMD) fix ./...
	GOOS=darwin golangci-lint run --fix
	GOOS=linux golangci-lint run --fix
	GOOS=windows golangci-lint run --fix
