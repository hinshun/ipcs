package command

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	cli "github.com/urfave/cli/v2"
)

// AppContext returns the context for a command. Should only be called once per
// command, near the start.
//
// This will ensure the namespace is picked up and set the timeout, if one is
// defined.
func AppContext(c *cli.Context) (context.Context, context.CancelFunc) {
	var (
		ctx       = context.Background()
		timeout   = c.Duration("timeout")
		namespace = c.String("namespace")
		cancel    context.CancelFunc
	)
	ctx = namespaces.WithNamespace(ctx, namespace)
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	return ctx, cancel
}

// NewClient returns a new containerd client
func NewClient(c *cli.Context, opts ...containerd.ClientOpt) (*containerd.Client, context.Context, context.CancelFunc, error) {
	client, err := containerd.New(c.String("containerd-address"), opts...)
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := AppContext(c)
	return client, ctx, cancel, nil
}
