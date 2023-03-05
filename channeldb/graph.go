package channeldb

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"

	"github.com/SeFo-Finance/obd-go-bindings/routing/route"

	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
)

var (
	// nodeBucket is a bucket which houses all the vertices or nodes within
	// the channel graph. This bucket has a single-sub bucket which adds an
	// additional index from pubkey -> alias. Within the top-level of this
	// bucket, the key space maps a node's compressed public key to the
	// serialized information for that node. Additionally, there's a
	// special key "source" which stores the pubkey of the source node. The
	// source node is used as the starting point for all graph/queries and
	// traversals. The graph is formed as a star-graph with the source node
	// at the center.
	//
	// maps: pubKey -> nodeInfo
	// maps: source -> selfPubKey
	nodeBucket = []byte("graph-node")

	// nodeUpdateIndexBucket is a sub-bucket of the nodeBucket. This bucket
	// will be used to quickly look up the "freshness" of a node's last
	// update to the network. The bucket only contains keys, and no values,
	// it's mapping:
	//
	// maps: updateTime || nodeID -> nil
	nodeUpdateIndexBucket = []byte("graph-node-update-index")

	// sourceKey is a special key that resides within the nodeBucket. The
	// sourceKey maps a key to the public key of the "self node".
	sourceKey = []byte("source")

	// aliasIndexBucket is a sub-bucket that's nested within the main
	// nodeBucket. This bucket maps the public key of a node to its
	// current alias. This bucket is provided as it can be used within a
	// future UI layer to add an additional degree of confirmation.
	aliasIndexBucket = []byte("alias")

	// edgeBucket is a bucket which houses all of the edge or channel
	// information within the channel graph. This bucket essentially acts
	// as an adjacency list, which in conjunction with a range scan, can be
	// used to iterate over all the incoming and outgoing edges for a
	// particular node. Key in the bucket use a prefix scheme which leads
	// with the node's public key and sends with the compact edge ID.
	// For each chanID, there will be two entries within the bucket, as the
	// graph is directed: nodes may have different policies w.r.t to fees
	// for their respective directions.
	//
	// maps: pubKey || chanID -> channel edge policy for node
	edgeBucket = []byte("graph-edge")

	// unknownPolicy is represented as an empty slice. It is
	// used as the value in edgeBucket for unknown channel edge policies.
	// Unknown policies are still stored in the database to enable efficient
	// lookup of incoming channel edges.
	unknownPolicy = []byte{}

	// chanStart is an array of all zero bytes which is used to perform
	// range scans within the edgeBucket to obtain all of the outgoing
	// edges for a particular node.
	chanStart [8]byte

	// edgeIndexBucket is an index which can be used to iterate all edges
	// in the bucket, grouping them according to their in/out nodes.
	// Additionally, the items in this bucket also contain the complete
	// edge information for a channel. The edge information includes the
	// capacity of the channel, the nodes that made the channel, etc. This
	// bucket resides within the edgeBucket above. Creation of an edge
	// proceeds in two phases: first the edge is added to the edge index,
	// afterwards the edgeBucket can be updated with the latest details of
	// the edge as they are announced on the network.
	//
	// maps: chanID -> pubKey1 || pubKey2 || restofEdgeInfo
	edgeIndexBucket = []byte("edge-index")

	// edgeUpdateIndexBucket is a sub-bucket of the main edgeBucket. This
	// bucket contains an index which allows us to gauge the "freshness" of
	// a channel's last updates.
	//
	// maps: updateTime || chanID -> nil
	edgeUpdateIndexBucket = []byte("edge-update-index")

	// channelPointBucket maps a channel's full outpoint (txid:index) to
	// its short 8-byte channel ID. This bucket resides within the
	// edgeBucket above, and can be used to quickly remove an edge due to
	// the outpoint being spent, or to query for existence of a channel.
	//
	// maps: outPoint -> chanID
	channelPointBucket = []byte("chan-index")

	// zombieBucket is a sub-bucket of the main edgeBucket bucket
	// responsible for maintaining an index of zombie channels. Each entry
	// exists within the bucket as follows:
	//
	// maps: chanID -> pubKey1 || pubKey2
	//
	// The chanID represents the channel ID of the edge that is marked as a
	// zombie and is used as the key, which maps to the public keys of the
	// edge's participants.
	zombieBucket = []byte("zombie-index")

	// disabledEdgePolicyBucket is a sub-bucket of the main edgeBucket bucket
	// responsible for maintaining an index of disabled edge policies. Each
	// entry exists within the bucket as follows:
	//
	// maps: <chanID><direction> -> []byte{}
	//
	// The chanID represents the channel ID of the edge and the direction is
	// one byte representing the direction of the edge. The main purpose of
	// this index is to allow pruning disabled channels in a fast way without
	// the need to iterate all over the graph.
	disabledEdgePolicyBucket = []byte("disabled-edge-policy-index")

	// graphMetaBucket is a top-level bucket which stores various meta-deta
	// related to the on-disk channel graph. Data stored in this bucket
	// includes the block to which the graph has been synced to, the total
	// number of channels, etc.
	graphMetaBucket = []byte("graph-meta")

	// pruneLogBucket is a bucket within the graphMetaBucket that stores
	// a mapping from the block height to the hash for the blocks used to
	// prune the graph.
	// Once a new block is discovered, any channels that have been closed
	// (by spending the outpoint) can safely be removed from the graph, and
	// the block is added to the prune log. We need to keep such a log for
	// the case where a reorg happens, and we must "rewind" the state of the
	// graph by removing channels that were previously confirmed. In such a
	// case we'll remove all entries from the prune log with a block height
	// that no longer exists.
	pruneLogBucket = []byte("prune-log")
)

const (
	// MaxAllowedExtraOpaqueBytes is the largest amount of opaque bytes that
	// we'll permit to be written to disk. We limit this as otherwise, it
	// would be possible for a node to create a ton of updates and slowly
	// fill our disk, and also waste bandwidth due to relaying.
	MaxAllowedExtraOpaqueBytes = 10000

	// feeRateParts is the total number of parts used to express fee rates.
	feeRateParts = 1e6
)

// channelMapKey is the key structure used for storing channel edge policies.
type channelMapKey struct {
	nodeKey route.Vertex
	chanID  [8]byte
}

// ChannelEdgeInfo represents a fully authenticated channel along with all its
// unique attributes. Once an authenticated channel announcement has been
// processed on the network, then an instance of ChannelEdgeInfo encapsulating
// the channels attributes is stored. The other portions relevant to routing
// policy of a channel are stored within a ChannelEdgePolicy for each direction
// of the channel.
type ChannelEdgeInfo struct {
	// ChannelID is the unique channel ID for the channel. The first 3
	// bytes are the block height, the next 3 the index within the block,
	// and the last 2 bytes are the output index for the channel.
	ChannelID uint64

	// ChainHash is the hash that uniquely identifies the chain that this
	// channel was opened within.
	//
	// TODO(roasbeef): need to modify db keying for multi-chain
	//  * must add chain hash to prefix as well
	ChainHash chainhash.Hash

	// NodeKey1Bytes is the raw public key of the first node.
	NodeKey1Bytes [33]byte
	nodeKey1      *btcec.PublicKey

	// NodeKey2Bytes is the raw public key of the first node.
	NodeKey2Bytes [33]byte
	nodeKey2      *btcec.PublicKey

	// BitcoinKey1Bytes is the raw public key of the first node.
	BitcoinKey1Bytes [33]byte
	bitcoinKey1      *btcec.PublicKey

	// BitcoinKey2Bytes is the raw public key of the first node.
	BitcoinKey2Bytes [33]byte
	bitcoinKey2      *btcec.PublicKey

	// Features is an opaque byte slice that encodes the set of channel
	// specific features that this channel edge supports.
	Features []byte

	// AuthProof is the authentication proof for this channel. This proof
	// contains a set of signatures binding four identities, which attests
	// to the legitimacy of the advertised channel.
	AuthProof *ChannelAuthProof

	// ChannelPoint is the funding outpoint of the channel. This can be
	// used to uniquely identify the channel within the channel graph.
	ChannelPoint wire.OutPoint

	/*obd update wxf
	AssetId >0 the unit is btcutil.Amount, else omnicore.Amount
	*/
	// Capacity is the total capacity of the channel, this is determined by
	// the value output in the outpoint that created this channel.
	Capacity lnwire.UnitPrec8
	AssetId  uint32

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	ExtraOpaqueData []byte

	db kvdb.Backend
}

// AddNodeKeys is a setter-like method that can be used to replace the set of
// keys for the target ChannelEdgeInfo.
func (c *ChannelEdgeInfo) AddNodeKeys(nodeKey1, nodeKey2, bitcoinKey1,
	bitcoinKey2 *btcec.PublicKey) {

	c.nodeKey1 = nodeKey1
	copy(c.NodeKey1Bytes[:], c.nodeKey1.SerializeCompressed())

	c.nodeKey2 = nodeKey2
	copy(c.NodeKey2Bytes[:], nodeKey2.SerializeCompressed())

	c.bitcoinKey1 = bitcoinKey1
	copy(c.BitcoinKey1Bytes[:], c.bitcoinKey1.SerializeCompressed())

	c.bitcoinKey2 = bitcoinKey2
	copy(c.BitcoinKey2Bytes[:], bitcoinKey2.SerializeCompressed())
}

// NodeKey1 is the identity public key of the "first" node that was involved in
// the creation of this channel. A node is considered "first" if the
// lexicographical ordering the its serialized public key is "smaller" than
// that of the other node involved in channel creation.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) NodeKey1() (*btcec.PublicKey, error) {
	if c.nodeKey1 != nil {
		return c.nodeKey1, nil
	}

	key, err := btcec.ParsePubKey(c.NodeKey1Bytes[:], btcec.S256())
	if err != nil {
		return nil, err
	}
	c.nodeKey1 = key

	return key, nil
}

// NodeKey2 is the identity public key of the "second" node that was
// involved in the creation of this channel. A node is considered
// "second" if the lexicographical ordering the its serialized public
// key is "larger" than that of the other node involved in channel
// creation.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) NodeKey2() (*btcec.PublicKey, error) {
	if c.nodeKey2 != nil {
		return c.nodeKey2, nil
	}

	key, err := btcec.ParsePubKey(c.NodeKey2Bytes[:], btcec.S256())
	if err != nil {
		return nil, err
	}
	c.nodeKey2 = key

	return key, nil
}

// BitcoinKey1 is the Bitcoin multi-sig key belonging to the first
// node, that was involved in the funding transaction that originally
// created the channel that this struct represents.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) BitcoinKey1() (*btcec.PublicKey, error) {
	if c.bitcoinKey1 != nil {
		return c.bitcoinKey1, nil
	}

	key, err := btcec.ParsePubKey(c.BitcoinKey1Bytes[:], btcec.S256())
	if err != nil {
		return nil, err
	}
	c.bitcoinKey1 = key

	return key, nil
}

// BitcoinKey2 is the Bitcoin multi-sig key belonging to the second
// node, that was involved in the funding transaction that originally
// created the channel that this struct represents.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) BitcoinKey2() (*btcec.PublicKey, error) {
	if c.bitcoinKey2 != nil {
		return c.bitcoinKey2, nil
	}

	key, err := btcec.ParsePubKey(c.BitcoinKey2Bytes[:], btcec.S256())
	if err != nil {
		return nil, err
	}
	c.bitcoinKey2 = key

	return key, nil
}

// OtherNodeKeyBytes returns the node key bytes of the other end of
// the channel.
func (c *ChannelEdgeInfo) OtherNodeKeyBytes(thisNodeKey []byte) (
	[33]byte, error) {

	switch {
	case bytes.Equal(c.NodeKey1Bytes[:], thisNodeKey):
		return c.NodeKey2Bytes, nil
	case bytes.Equal(c.NodeKey2Bytes[:], thisNodeKey):
		return c.NodeKey1Bytes, nil
	default:
		return [33]byte{}, fmt.Errorf("node not participating in this channel")
	}
}

// ChannelAuthProof is the authentication proof (the signature portion) for a
// channel. Using the four signatures contained in the struct, and some
// auxiliary knowledge (the funding script, node identities, and outpoint) nodes
// on the network are able to validate the authenticity and existence of a
// channel. Each of these signatures signs the following digest: chanID ||
// nodeID1 || nodeID2 || bitcoinKey1|| bitcoinKey2 || 2-byte-feature-len ||
// features.
type ChannelAuthProof struct {
	// nodeSig1 is a cached instance of the first node signature.
	nodeSig1 *btcec.Signature

	// NodeSig1Bytes are the raw bytes of the first node signature encoded
	// in DER format.
	NodeSig1Bytes []byte

	// nodeSig2 is a cached instance of the second node signature.
	nodeSig2 *btcec.Signature

	// NodeSig2Bytes are the raw bytes of the second node signature
	// encoded in DER format.
	NodeSig2Bytes []byte

	// bitcoinSig1 is a cached instance of the first bitcoin signature.
	bitcoinSig1 *btcec.Signature

	// BitcoinSig1Bytes are the raw bytes of the first bitcoin signature
	// encoded in DER format.
	BitcoinSig1Bytes []byte

	// bitcoinSig2 is a cached instance of the second bitcoin signature.
	bitcoinSig2 *btcec.Signature

	// BitcoinSig2Bytes are the raw bytes of the second bitcoin signature
	// encoded in DER format.
	BitcoinSig2Bytes []byte
}

// Node1Sig is the signature using the identity key of the node that is first
// in a lexicographical ordering of the serialized public keys of the two nodes
// that created the channel.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the signature if absolutely necessary.
func (c *ChannelAuthProof) Node1Sig() (*btcec.Signature, error) {
	if c.nodeSig1 != nil {
		return c.nodeSig1, nil
	}

	sig, err := btcec.ParseSignature(c.NodeSig1Bytes, btcec.S256())
	if err != nil {
		return nil, err
	}

	c.nodeSig1 = sig

	return sig, nil
}

// Node2Sig is the signature using the identity key of the node that is second
// in a lexicographical ordering of the serialized public keys of the two nodes
// that created the channel.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the signature if absolutely necessary.
func (c *ChannelAuthProof) Node2Sig() (*btcec.Signature, error) {
	if c.nodeSig2 != nil {
		return c.nodeSig2, nil
	}

	sig, err := btcec.ParseSignature(c.NodeSig2Bytes, btcec.S256())
	if err != nil {
		return nil, err
	}

	c.nodeSig2 = sig

	return sig, nil
}

// BitcoinSig1 is the signature using the public key of the first node that was
// used in the channel's multi-sig output.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the signature if absolutely necessary.
func (c *ChannelAuthProof) BitcoinSig1() (*btcec.Signature, error) {
	if c.bitcoinSig1 != nil {
		return c.bitcoinSig1, nil
	}

	sig, err := btcec.ParseSignature(c.BitcoinSig1Bytes, btcec.S256())
	if err != nil {
		return nil, err
	}

	c.bitcoinSig1 = sig

	return sig, nil
}

// BitcoinSig2 is the signature using the public key of the second node that
// was used in the channel's multi-sig output.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the signature if absolutely necessary.
func (c *ChannelAuthProof) BitcoinSig2() (*btcec.Signature, error) {
	if c.bitcoinSig2 != nil {
		return c.bitcoinSig2, nil
	}

	sig, err := btcec.ParseSignature(c.BitcoinSig2Bytes, btcec.S256())
	if err != nil {
		return nil, err
	}

	c.bitcoinSig2 = sig

	return sig, nil
}

// IsEmpty check is the authentication proof is empty Proof is empty if at
// least one of the signatures are equal to nil.
func (c *ChannelAuthProof) IsEmpty() bool {
	return len(c.NodeSig1Bytes) == 0 ||
		len(c.NodeSig2Bytes) == 0 ||
		len(c.BitcoinSig1Bytes) == 0 ||
		len(c.BitcoinSig2Bytes) == 0
}

// genMultiSigP2WSH generates the p2wsh'd multisig script for 2 of 2 pubkeys.
func genMultiSigP2WSH(aPub, bPub []byte) ([]byte, error) {
	if len(aPub) != 33 || len(bPub) != 33 {
		return nil, fmt.Errorf("pubkey size error. Compressed " +
			"pubkeys only")
	}

	// Swap to sort pubkeys if needed. Keys are sorted in lexicographical
	// order. The signatures within the scriptSig must also adhere to the
	// order, ensuring that the signatures for each public key appears in
	// the proper order on the stack.
	if bytes.Compare(aPub, bPub) == 1 {
		aPub, bPub = bPub, aPub
	}

	// First, we'll generate the witness script for the multi-sig.
	bldr := txscript.NewScriptBuilder()
	bldr.AddOp(txscript.OP_2)
	bldr.AddData(aPub) // Add both pubkeys (sorted).
	bldr.AddData(bPub)
	bldr.AddOp(txscript.OP_2)
	bldr.AddOp(txscript.OP_CHECKMULTISIG)
	witnessScript, err := bldr.Script()
	if err != nil {
		return nil, err
	}

	// With the witness script generated, we'll now turn it into a p2sh
	// script:
	//  * OP_0 <sha256(script)>
	bldr = txscript.NewScriptBuilder()
	bldr.AddOp(txscript.OP_0)
	scriptHash := sha256.Sum256(witnessScript)
	bldr.AddData(scriptHash[:])

	return bldr.Script()
}

// EdgePoint couples the outpoint of a channel with the funding script that it
// creates. The FilteredChainView will use this to watch for spends of this
// edge point on chain. We require both of these values as depending on the
// concrete implementation, either the pkScript, or the out point will be used.
type EdgePoint struct {
	// FundingPkScript is the p2wsh multi-sig script of the target channel.
	FundingPkScript []byte

	// OutPoint is the outpoint of the target channel.
	OutPoint wire.OutPoint
}

// String returns a human readable version of the target EdgePoint. We return
// the outpoint directly as it is enough to uniquely identify the edge point.
func (e *EdgePoint) String() string {
	return e.OutPoint.String()
}
