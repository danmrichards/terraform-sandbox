GOARCH=amd64

build: linux darwin windows

linux:
	GO111MODULE=on CGO_ENABLED=0 GOARCH=${GOARCH} GOOS=linux go build -o ./bin/tfsandbox-linux-${GOARCH}

darwin:
	GO111MODULE=on CGO_ENABLED=0 GOARCH=${GOARCH} GOOS=darwin go build -o ./bin/tfsandbox-darwin-${GOARCH}

windows:
	GO111MODULE=on CGO_ENABLED=0 GOARCH=${GOARCH} GOOS=windows go build -o ./bin/tfsandbox-windows-${GOARCH}.exe

.PHONY: build
