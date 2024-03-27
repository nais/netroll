
.PHONY:
all: test build

test:
	go run github.com/onsi/ginkgo/v2/ginkgo -r --randomize-all --randomize-suites --fail-on-pending --keep-going --github-output -v

build:
	go build -a -installsuffix cgo -o bin/netroll cmd/netroll/main.go

local:
	go run cmd/netroll/main.go -bind-address 127.0.0.1:8080
