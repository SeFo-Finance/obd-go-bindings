package channeldb

import (
	"errors"
	"time"
)

var (
	// peersBucket is the name of a top level bucket in which we store
	// information about our peers. Information for different peers is
	// stored in buckets keyed by their public key.
	//
	//
	// peers-bucket
	//      |
	//      |-- <peer-pubkey>
	//      |        |--flap-count-key: <ts><flap count>
	//      |
	//      |-- <peer-pubkey>
	//      |        |--flap-count-key: <ts><flap count>
	peersBucket = []byte("peers-bucket")

	// flapCountKey is a key used in the peer pubkey sub-bucket that stores
	// the timestamp of a peer's last flap count and its all time flap
	// count.
	flapCountKey = []byte("flap-count")
)

var (
	// ErrNoPeerBucket is returned when we try to read entries for a peer
	// that is not tracked.
	ErrNoPeerBucket = errors.New("peer bucket not found")
)

// FlapCount contains information about a peer's flap count.
type FlapCount struct {
	// Count provides the total flap count for a peer.
	Count uint32

	// LastFlap is the timestamp of the last flap recorded for a peer.
	LastFlap time.Time
}
