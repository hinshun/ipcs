package main

import (
	"context"
	"fmt"
	"net"
	"os"

	contentapi "github.com/containerd/containerd/api/services/content/v1"
	"github.com/containerd/containerd/services/content/contentserver"
	"github.com/hinshun/ipcs"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func main() {
	err := run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Provide a unix address to listen to, this will be the `address`
	// in the `proxy_plugin` configuration.
	// The root will be used to store the snapshots.
	if len(args) < 3 {
		return errors.Errorf("invalid args: usage: %s <unix addr> <root>\n", args[0])
	}

	p, err := ipcs.New(context.Background(), args[2], 0)
	if err != nil {
		return errors.Wrap(err, "failed to create ipcs content store")
	}

	// Convert the content store to a gRPC service.
	service := contentserver.New(p)

	// Create a gRPC server.
	rpc := grpc.NewServer()

	// Register the service with the gRPC server.
	contentapi.RegisterContentServer(rpc, service)

	// Register the peer's resolver service with the gRPC server.
	ipcs.RegisterResolverServer(rpc, p)

	// Listen and serve.
	os.Remove(args[1])
	l, err := net.Listen("unix", args[1])
	if err != nil {
		return err
	}
	defer l.Close()

	fmt.Printf("Identity generated %s\n", p.Host().ID())
	fmt.Printf("GRPC Server listening on %s\n", args[1])
	for _, ma := range p.Host().Addrs() {
		fmt.Printf("Libp2p listening on %s\n", ma)
	}
	return rpc.Serve(l)
}
