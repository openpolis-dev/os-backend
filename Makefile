all: docs build

.PHONY: docs clean test docker

GO_INTERNAL_SRC = $(wildcard internal/*.go internal/*/*.go internal/*/*/*.go)
GO_CMD_SRC = $(wildcard cmd/*.go)
OUTPUT_BINS = $(patsubst cmd/%.go, bin/%, $(wildcard cmd/*))

PB_FILES = $(wildcard proto/*.proto)
DOCS_FILES = docs/swagger.json

build: $(OUTPUT_BINS)
docs: ${DOCS_FILES}

${DOCS_FILES}: $(GO_INTERNAL_SRC) $(GO_CMD_SRC)
	swag init -g ../../cmd/apiserver.go -d internal/api internal/model

internal/models/%.pb.go: proto/%.proto
	protoc --proto_path=proto --go_out=internal/models --go_opt=paths=source_relative $<

bin/%: ./cmd/%.go $(GO_INTERNAL_SRC)
	go build -o $@ ./$(shell dirname $<)

docker:
	docker build -t $(DOCKER_IMG_NAME) .

test:
	go test -v -cover ./...

clean:
	-rm -rf bin/

