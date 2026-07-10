.PHONY: build test vet fmt run clean check

build:
	go build -o toyblockchain.exe .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check: fmt vet test

run:
	go run . \
		-difficulty=4 \
		-mining-timeout=15s \
		-max-attempts=5000000 \
		-max-nonce=10000000 \
		-data=data/chain.json \
		-wallets=data/wallets.json

clean:
	rm -f toyblockchain
	rm -f toyblockchain.exe
	rm -f data/chain.json
	rm -f data/chain.json.lock
	rm -f data/chain.json.tmp
	rm -f data/wallets.json
	rm -f data/wallets.json.tmp