.PHONY: generate generate-containerlab test build

generate-containerlab:
	bash scripts/generate-containerlab.sh

generate:
	PATH="$${HOME}/go/bin:$${PATH}" protoc -I . --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/v1/labcontainers.proto
	protoc -I api/v1 --python_out=python/labcontainers api/v1/labcontainers.proto

test:
	go test ./...
	python3 -m py_compile python/labcontainers/*.py
	python3 -m unittest discover -s python/tests -v

build:
	mkdir -p bin
	go build -trimpath -o bin/labd ./cmd/labd
	go build -trimpath -o bin/labctl ./cmd/labctl
	CGO_ENABLED=0 go build -trimpath -o bin/labcontainers-guest ./cmd/labcontainers-guest
