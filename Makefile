GOOS?=linux
GOARCH?=amd64

convert:
	@IPFS_PATH=./tmp/ipfs go run ./cmd/convert docker.io/library/alpine:latest localhost:5000/library/alpine:p2p

registry:
	@docker run --rm -it --name registry -p 5000:5000 registry:latest

ipcs:
	@mkdir -p ./tmp/containerd/root/plugins
	@go build -buildmode=plugin -o ./tmp/containerd/root/plugins/ipcs-$(GOOS)-$(GOARCH).so cmd/ipcs/main.go

bin/containerd:
	@mkdir -p ./bin
	@go build -o ./bin/containerd ./cmd/containerd

containerd: bin/containerd ipcs
	@mkdir -p ./tmp
	@IPFS_PATH=./tmp/ipfs rootlesskit --net=slirp4netns --copy-up=/etc \
	  --state-dir=./tmp/rootlesskit-containerd \
	    ./bin/containerd -l debug --config ./cmd/containerd/config.toml

ipfs:
	@mkdir -p ./tmp
	@IPFS_PATH=./tmp/ipfs ipfs daemon --init

clean:
	@rm -rf ./tmp ./bin

.PHONY: convert registry ipcs containerd
