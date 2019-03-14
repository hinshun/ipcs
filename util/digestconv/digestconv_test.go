package digestconv

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipfs-util"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
)

func TestDigestToCid(t *testing.T) {
	data := []byte("foobar")
	expected := cid.NewCidV0(util.Hash(data))
	actual, err := DigestToCid(digest.FromBytes(data))
	require.NoError(t, err)
	require.Equal(t, expected.String(), actual.String())
}

func TestCidToDigest(t *testing.T) {
	data := []byte("foobar")
	expected := digest.FromBytes(data)
	actual, err := CidToDigest(cid.NewCidV0(util.Hash(data)))
	require.NoError(t, err)
	require.Equal(t, expected.String(), actual.String())
}
