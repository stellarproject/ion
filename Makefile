PACKAGES=$(shell go list ./... | grep -v /vendor/)
REVISION=$(shell git rev-parse HEAD)
GO_LDFLAGS=-s -w -X github.com/stellarproject/ion/version.Version=$(REVISION)
BINARY=ion

all:
	rm -f ${BINARY}
	go build -v -ldflags '${GO_LDFLAGS}' -o ${BINARY} .

vab:
	rm -f ${BINARY}
	vab build --local

image:
	vab build -p --ref docker.io/stellarproject/ion:latest

static:
	CGO_ENALBED=0 go build -v -ldflags '${GO_LDFLAGS} -extldflags "-static"' -o ${BINARY} .
