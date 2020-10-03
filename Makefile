convert:
	@go run ./cmd/convert alpine

compare:
	@go run ./cmd/compare docker.io/library/ubuntu:xenial docker.io/titusoss/ubuntu:latest

ipcsd:
	@mkdir -p ./tmp/ipcsd
	@go run ./cmd/ipcsd ./tmp/ipcsd/ipcsd.sock ./tmp/ipcsd

containerd:
	@mkdir -p ./tmp
	@./bin/rootlesskit \
		--copy-up=/etc \
		--copy-up=/run \
                --state-dir=/run/user/1001/rootlesskit-containerd \
                sh -c "rm -f /run/containerd; exec ./bin/containerd -config ./containerd.toml"

nsenter:
	@nsenter -U --preserve-credentials -m -t $$(cat /run/user/1001/rootlesskit-containerd/child_pid)
	  	    
clean:
	@rm -rf ./tmp ./bin

.PHONY: convert compare ipcsd rootless-containerd containerd
