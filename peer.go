package ipcs

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	bitswap "github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	blockservice "github.com/ipfs/go-blockservice"
	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	config "github.com/ipfs/go-ipfs-config"
	files "github.com/ipfs/go-ipfs-files"
	provider "github.com/ipfs/go-ipfs-provider"
	"github.com/ipfs/go-ipfs-provider/queue"
	"github.com/ipfs/go-ipfs-provider/simple"
	"github.com/ipfs/go-ipfs/keystore"
	"github.com/ipfs/go-ipfs/namesys"
	"github.com/ipfs/go-ipfs/namesys/resolve"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	ipns "github.com/ipfs/go-ipns"
	merkledag "github.com/ipfs/go-merkledag"
	ipfspath "github.com/ipfs/go-path"
	"github.com/ipfs/go-path/resolver"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipfs/go-unixfs/importer/balanced"
	"github.com/ipfs/go-unixfs/importer/helpers"
	uio "github.com/ipfs/go-unixfs/io"
	iface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	noise "github.com/libp2p/go-libp2p-noise"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	quic "github.com/libp2p/go-libp2p-quic-transport"
	record "github.com/libp2p/go-libp2p-record"
	tls "github.com/libp2p/go-libp2p-tls"
	yamux "github.com/libp2p/go-libp2p-yamux"
	multihash "github.com/multiformats/go-multihash"
	"github.com/pkg/errors"
)

func init() {
	ipld.Register(cid.DagProtobuf, merkledag.DecodeProtobufBlock)
	ipld.Register(cid.DagCBOR, cbor.DecodeBlock) // need to decode CBOR
	ipld.Register(cid.Raw, merkledag.DecodeRawBlock)
}

var (
	ReprovideInterval = 12 * time.Hour
)

type Peer struct {
	host     host.Host
	routing  routing.ContentRouting
	namesys  namesys.NameSystem
	provider provider.System
	dserv    ipld.DAGService
	dstore   datastore.Batching
	bstore   blockstore.Blockstore
	bserv    blockservice.BlockService
	bswap    *bitswap.Bitswap
	kstore   keystore.Keystore
}

func New(ctx context.Context, addr, root string) (*Peer, error) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate private key")
	}

	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, err
	}

	pstore := pstoremem.NewPeerstore()
	err = pstore.AddPubKey(id, pub)
	if err != nil {
		return nil, err
	}

	err = pstore.AddPrivKey(id, priv)
	if err != nil {
		return nil, err
	}

	dstore, err := badger.NewDatastore(root, &badger.DefaultOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create datastore")
	}

	kstore, err := keystore.NewFSKeystore(filepath.Join(root, "keystore"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create keystore")
	}

	validator := record.NamespacedValidator{
		"pk":   record.PublicKeyValidator{},
		"ipns": ipns.Validator{KeyBook: pstore},
	}

	// bootstrapAddrs := append(config.DefaultBootstrapAddresses, "/ip4/192.168.1.97/udp/4001/quic/p2p/12D3KooWQD2jNpbXJGkoqyTZ1nDyrFbZwTUnX6Tc6AT5HrmG7xMZ")
	bootstrapAddrs := []string{"/ip4/10.0.0.2/udp/35671/quic/p2p/12D3KooWGAvEtNZn6zfYVzydKi8AwJNMFGzpMmnjwV1dRHctMNTd"}
	bootstrapPeers, err := config.ParseBootstrapPeers(bootstrapAddrs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bootstrap peers")
	}

	var r *dual.DHT
	newRouting := func(h host.Host) (routing.PeerRouting, error) {
		var err error
		r, err = dual.New(
			ctx, h,
			dual.DHTOption(
				dht.Concurrency(10),
				dht.Mode(dht.ModeAuto),
				dht.Datastore(dstore),
				dht.Validator(validator),
			),
			dual.WanDHTOption(dht.BootstrapPeers(bootstrapPeers...)),
		)
		return r, err
	}

	h, err := libp2p.New(
		ctx,
		libp2p.Identity(pstore.PrivKey(id)),
		libp2p.Peerstore(pstore),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Security(tls.ID, tls.New),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.ListenAddrStrings(addr),
		libp2p.Routing(newRouting),

		// -------------
		// NAT Traversal
		// -------------

		// Attempt to create port mapping on router via UPnP.
		libp2p.NATPortMap(),
		// Detects if behind NAT, if so, find & announce public relays.
		libp2p.EnableAutoRelay(),
		// Provide service for other peers to test reachability.
		libp2p.EnableNATService(),
	)

	bstore, err := NewBlockstore(ctx, dstore)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create blockstore")
	}

	bswapnet := network.NewFromIpfsHost(h, r)
	rem := bitswap.New(ctx, bswapnet, bstore)

	bswap, ok := rem.(*bitswap.Bitswap)
	if !ok {
		return nil, errors.New("expected to be able to cast exchange interface to bitswap")
	}

	bserv := blockservice.New(bstore, rem)

	provider, err := NewProviderSystem(ctx, dstore, bstore, r)
	if err != nil {
		bserv.Close()
		return nil, errors.Wrap(err, "failed to create provider provider")
	}

	go func() {
		provider.Run()

		select {
		case <-ctx.Done():
			err := provider.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to close provider: %q\n", err)
			}

			err = bserv.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to close block service: %q\n", err)
			}

			err = pstore.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to close peer store: %q\n", err)
			}
		}
	}()

	dserv := merkledag.NewDAGService(bserv)
	ns := namesys.NewNameSystem(r, dstore, 0)

	return &Peer{
		host:     h,
		routing:  r,
		namesys:  ns,
		provider: provider,
		dserv:    dserv,
		bswap:    bswap,
		bserv:    bserv,
		bstore:   bstore,
		kstore:   kstore,
		dstore:   dstore,
	}, nil
}

func (p *Peer) Host() host.Host {
	return p.host
}

func (p *Peer) DAGService() ipld.DAGService {
	return p.dserv
}

func (p *Peer) Add(ctx context.Context, r io.Reader) (ipld.Node, error) {
	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return nil, errors.Wrap(err, "unrecognized CID version")
	}

	hashName := "sha2-256"
	hashFuncCode, ok := multihash.Names[hashName]
	if !ok {
		return nil, errors.Wrapf(err, "unrecognized hash function %q", hashName)
	}
	prefix.MhType = hashFuncCode

	buf := ipld.NewBufferedDAG(ctx, p.dserv)
	dbp := helpers.DagBuilderParams{
		Dagserv:    buf,
		RawLeaves:  false,
		Maxlinks:   helpers.DefaultLinksPerBlock,
		CidBuilder: &prefix,
	}

	chnk, err := chunker.FromString(r, "size-262144")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create chunker")
	}

	dbh, err := dbp.New(chnk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create dag builder")
	}

	nd, err := balanced.Layout(dbh)
	if err != nil {
		return nil, err
	}

	return nd, buf.Commit()
}

func (p *Peer) Get(ctx context.Context, ref string) (ipld.Node, error) {
	cpath := corepath.New(ref)
	ipath := ipfspath.Path(cpath.String())
	ipath, err := resolve.ResolveIPNS(ctx, p.namesys, ipath)
	if err != nil {
		return nil, err
	}

	var resolveOnce resolver.ResolveOnce
	switch ipath.Segments()[0] {
	case "ipfs":
		resolveOnce = uio.ResolveUnixfsOnce
	case "ipld":
		resolveOnce = resolver.ResolveSingle
	default:
		return nil, fmt.Errorf("unsupported path namespace: %s", cpath.Namespace())
	}

	r := &resolver.Resolver{
		DAG:         p.dserv,
		ResolveOnce: resolveOnce,
	}

	c, _, err := r.ResolveToLastNode(ctx, ipath)
	if err != nil {
		return nil, err
	}

	return p.dserv.Get(ctx, c)
}

func (p *Peer) GetFile(ctx context.Context, ref string) (files.File, error) {
	nd, err := p.Get(ctx, ref)
	if err != nil {
		return nil, err
	}

	f, err := unixfile.NewUnixfsFile(ctx, p.dserv, nd)
	if err != nil {
		return nil, err
	}

	var file files.File
	switch f := f.(type) {
	case files.File:
		file = f
	case files.Directory:
		return nil, iface.ErrIsDir
	default:
		return nil, iface.ErrNotSupported
	}

	return file, nil
}

func NewBlockstore(ctx context.Context, ds datastore.Batching) (blockstore.Blockstore, error) {
	bs := blockstore.NewBlockstore(ds)
	bs = blockstore.NewIdStore(bs)

	var err error
	bs, err = blockstore.CachedBlockstore(ctx, bs, blockstore.DefaultCacheOpts())
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func NewProviderSystem(ctx context.Context, ds datastore.Batching, bs blockstore.Blockstore, r routing.ContentRouting) (provider.System, error) {
	queue, err := queue.NewQueue(ctx, "repro", ds)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new queue")
	}

	prov := simple.NewProvider(ctx, queue, r)
	reprov := simple.NewReprovider(ctx, ReprovideInterval, r, simple.NewBlockstoreProvider(bs))
	system := provider.NewSystem(prov, reprov)
	return system, nil
}
