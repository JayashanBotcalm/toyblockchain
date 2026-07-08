.PHONY: build test vet fmt run clean

build:
	go build -o toyblockchain .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

run: build
	./toyblockchain -difficulty=3 -data=data/chain.json

clean:
	rm -f toyblockchain
	rm -f data/chain.json
