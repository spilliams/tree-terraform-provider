.PHONY: build
build: bin/tree

.PHONY: tidy
tidy:
	go mod tidy

bin/tree:
	GOTOOLCHAIN=go1.23.2 go build -o bin/tree example/main.go

.PHONY: test
test:
	GOTOOLCHAIN=go1.23.2 go test -v -cover -timeout=120s -parallel=10 ./...
