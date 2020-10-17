package ipcs

import (
	"context"
	"io"
	"io/ioutil"
	"sync"
	"time"

	contentapi "github.com/containerd/containerd/api/services/content/v1"
	"github.com/containerd/containerd/content"
	contentproxy "github.com/containerd/containerd/content/proxy"
	"github.com/containerd/containerd/pkg/dialer"
	"github.com/containerd/containerd/remotes"
	"github.com/hinshun/ipcs/defaults"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

type Client struct {
	conn      *grpc.ClientConn
	connMu    sync.Mutex
	connector func() (*grpc.ClientConn, error)
}

func NewClient(address string) (*Client, error) {
	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = 3 * time.Second
	connParams := grpc.ConnectParams{
		Backoff: backoffConfig,
	}
	gopts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithConnectParams(connParams),
		grpc.WithContextDialer(dialer.ContextDialer),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(defaults.DefaultMaxRecvMsgSize)),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(defaults.DefaultMaxSendMsgSize)),
	}
	connector := func() (*grpc.ClientConn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, dialer.DialAddress(address), gopts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to dial %q", address)
		}
		return conn, nil
	}

	conn, err := connector()
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:      conn,
		connector: connector,
	}, nil
}

// Reconnect re-establishes the GRPC connection to the contentd daemon
func (c *Client) Reconnect() error {
	if c.connector == nil {
		return errors.Errorf("unable to reconnect to contentd, no connector available")
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	c.conn.Close()
	conn, err := c.connector()
	if err != nil {
		return err
	}

	c.conn = conn
	return nil
}

func (c *Client) Resolver() remotes.Resolver {
	return &resolverProxy{
		store:    contentproxy.NewContentStore(contentapi.NewContentClient(c.conn)),
		resolver: NewResolverClient(c.conn),
	}
}

// func (c *Client) Keystore() Keystore {
// 	return &keystoreClient{
// 		cln: NewKeystoreClient(c.conn),
// 	}
// }

// type Keystore interface {
// 	Add(ctx context.Context, name, pubKey string) error
// 	Generate(ctx context.Context, name, keyType string, size int) (pubKey string, err error)
// }

type resolverProxy struct {
	store    content.Store
	resolver ResolverClient
}

func (rp *resolverProxy) Resolve(ctx context.Context, ref string) (name string, desc ocispec.Descriptor, err error) {
	resp, err := rp.resolver.Resolve(ctx, &ResolveRequest{Ref: ref})
	if err != nil {
		return
	}

	resolved := resp.Resolved
	return resolved.Name, ocispec.Descriptor{
		MediaType: resolved.Target.MediaType,
		Digest:    resolved.Target.Digest,
		Size:      resolved.Target.Size_,
	}, nil
}

func (rp *resolverProxy) Fetcher(ctx context.Context, ref string) (remotes.Fetcher, error) {
	return &providerFetcher{rp.store}, nil
}

// Pusher returns a new pusher for the provided reference
func (rp *resolverProxy) Pusher(ctx context.Context, ref string) (remotes.Pusher, error) {
	panic("unimplemented")
	return nil, nil
}

type providerFetcher struct {
	provider content.Provider
}

func (pf *providerFetcher) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	ra, err := pf.provider.ReaderAt(ctx, desc)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(content.NewReader(ra)), nil
}
