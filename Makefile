GOOS?=linux
GOARCH?=amd64

convert:
	@IPFS_PATH=./tmp/ipfs go run ./cmd/convert docker.io/library/alpine:latest localhost:5000/library/alpine:p2p

compare:
	@IPFS_PATH=./tmp/ipfs go run ./cmd/compare docker.io/library/ubuntu:xenial docker.io/titusoss/ubuntu:latest

ipcs:
	@mkdir -p ./tmp/ipcs
	@./bin/ipcs ./tmp/ipcs/ipcs.sock ./tmp/ipcs

containerd:
	@mkdir -p ./tmp
	@IPFS_PATH=./tmp/ipfs ./bin/rootlesskit --copy-up=/etc \
	  --state-dir=./tmp/rootlesskit-containerd \
	    ./bin/containerd -l debug --config ./containerd.toml
	    
clean:
	@rm -rf ./tmp ./bin

.PHONY: convert ipcs containerd
