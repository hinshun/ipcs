convert:
	@go run ./cmd/convert alpine

compare:
	@go run ./cmd/compare docker.io/library/ubuntu:xenial docker.io/titusoss/ubuntu:latest

contentd:
	@mkdir -p /run/user/1001/contentd
	@mkdir -p ./tmp/contentd
	@go run ./cmd/contentd /ip4/10.0.0.1/udp/0/quic ./tmp/contentd /run/user/1001/contentd/contentd.sock

containerd:
	@mkdir -p ./tmp
	@./bin/rootlesskit \
		--copy-up=/etc \
		--copy-up=/run \
                --state-dir=/run/user/1001/rootlesskit-containerd \
                sh -c "rm -f /run/containerd; exec ./bin/containerd -config ./containerd.toml"

nsenter:
	@nsenter --preserve-credentials -U -m -w -t $$(cat /run/user/1001/rootlesskit-containerd/child_pid)
	  	    
clean:
	@rm -rf ./tmp ./bin

.PHONY: convert compare contentd rootless-containerd containerd
