.DEFAULT_GOAL := all
all: vendor vet security test build

build:
	docker build --build-arg BUILD="${CIRCLE_SHA1}" -t make-slowgest-api .

generate:
	go generate ./...

security:
	go get github.com/securego/gosec/cmd/gosec/...
	gosec -exclude=G104 ./...

test:
	docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit
	docker-compose -f docker-compose.test.yml down

vendor:
	GO111MODULE=on go mod vendor

vet:
	go vet -mod=vendor ./...
	
.PHONY: all build generate security test vendor vet
