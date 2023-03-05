package htlcswitch

import (
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/contractcourt"
	"github.com/lightningnetwork/lnd/ticker"
	prand "math/rand"
	"time"

	"github.com/lightningnetwork/lnd/lnwallet/chainfee"

	"github.com/SeFo-Finance/obd-go-bindings/htlcswitch/hodl"
	"github.com/SeFo-Finance/obd-go-bindings/lnpeer"
	"github.com/SeFo-Finance/obd-go-bindings/omnicore"

	"github.com/SeFo-Finance/obd-go-bindings/htlcswitch/hop"
	"github.com/SeFo-Finance/obd-go-bindings/lnwallet"
	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
)

func init() {
	prand.Seed(time.Now().UnixNano())
}

const (
	// DefaultMaxOutgoingCltvExpiry is the maximum outgoing time lock that
	// the node accepts for forwarded payments. The value is relative to the
	// current block height. The reason to have a maximum is to prevent
	// funds getting locked up unreasonably long. Otherwise, an attacker
	// willing to lock its own funds too, could force the funds of this node
	// to be locked up for an indefinite (max int32) number of blocks.
	//
	// The value 2016 corresponds to on average two weeks worth of blocks
	// and is based on the maximum number of hops (20), the default CLTV
	// delta (40), and some extra margin to account for the other lightning
	// implementations and past lnd versions which used to have a default
	// CLTV delta of 144.
	DefaultMaxOutgoingCltvExpiry = 2016

	// DefaultMinLinkFeeUpdateTimeout represents the minimum interval in
	// which a link should propose to update its commitment fee rate.
	DefaultMinLinkFeeUpdateTimeout = 10 * time.Minute

	// DefaultMaxLinkFeeUpdateTimeout represents the maximum interval in
	// which a link should propose to update its commitment fee rate.
	DefaultMaxLinkFeeUpdateTimeout = 60 * time.Minute

	// DefaultMaxLinkFeeAllocation is the highest allocation we'll allow
	// a channel's commitment fee to be of its balance. This only applies to
	// the initiator of the channel.
	DefaultMaxLinkFeeAllocation float64 = 0.5
)

func (fpCfg *ForwardingPolicyCfg) LoadCfg(assetId uint32) (fp *ForwardingPolicy) {
	fp = new(ForwardingPolicy)
	fp.TimeLockDelta = fpCfg.TimeLockDelta
	fp.BaseFee = fpCfg.BaseFee
	if assetId == lnwire.BtcAssetId {
		fp.MinHTLCOut = lnwire.UnitPrec11(fpCfg.MinHTLCOut)
		fp.MaxHTLC = lnwire.UnitPrec11(fpCfg.MaxHTLC)
		fp.FeeRate = lnwire.UnitPrec11(fpCfg.FeeRate)
		return fp
	}
	fp.MinHTLCOut = lnwire.UnitPrec11(fpCfg.AssetMinHTLCOut)
	fp.MaxHTLC = lnwire.UnitPrec11(fpCfg.AssetMaxHTLC)
	fp.FeeRate = lnwire.UnitPrec11(fpCfg.AssetFeeRate)
	return fp
}

type ForwardingPolicyCfg struct {
	// MinHTLC is the smallest HTLC that is to be forwarded.
	MinHTLCOut      lnwire.MilliSatoshi
	AssetMinHTLCOut omnicore.Amount

	// MaxHTLC is the largest HTLC that is to be forwarded.
	MaxHTLC      lnwire.MilliSatoshi
	AssetMaxHTLC omnicore.Amount

	// BaseFee is the base fee, expressed in milli-satoshi that must be
	// paid for each incoming HTLC. This field, combined with FeeRate is
	// used to compute the required fee for a given HTLC.
	BaseFee lnwire.MilliSatoshi

	// FeeRate is the fee rate, expressed in milli-satoshi that must be
	// paid for each incoming HTLC. This field combined with BaseFee is
	// used to compute the required fee for a given HTLC.
	FeeRate      lnwire.MilliSatoshi
	AssetFeeRate omnicore.Amount

	// TimeLockDelta is the absolute time-lock value, expressed in blocks,
	// that will be subtracted from an incoming HTLC's timelock value to
	// create the time-lock value for the forwarded outgoing HTLC. The
	// following constraint MUST hold for an HTLC to be forwarded:
	//
	//  * incomingHtlc.timeLock - timeLockDelta = fwdInfo.OutgoingCTLV
	//
	//    where fwdInfo is the forwarding information extracted from the
	//    per-hop payload of the incoming HTLC's onion packet.
	TimeLockDelta uint32

	// TODO(roasbeef): add fee module inside of switch
}

// ForwardingPolicy describes the set of constraints that a given ChannelLink
// is to adhere to when forwarding HTLC's. For each incoming HTLC, this set of
// constraints will be consulted in order to ensure that adequate fees are
// paid, and our time-lock parameters are respected. In the event that an
// incoming HTLC violates any of these constraints, it is to be _rejected_ with
// the error possibly carrying along a ChannelUpdate message that includes the
// latest policy.
type ForwardingPolicy struct {

	// MinHTLC is the smallest HTLC that is to be forwarded.
	MinHTLCOut lnwire.UnitPrec11

	// MaxHTLC is the largest HTLC that is to be forwarded.
	MaxHTLC lnwire.UnitPrec11

	// BaseFee is the base fee, expressed in milli-satoshi that must be
	// paid for each incoming HTLC. This field, combined with FeeRate is
	// used to compute the required fee for a given HTLC.
	BaseFee lnwire.MilliSatoshi

	// FeeRate is the fee rate, expressed in milli-satoshi that must be
	// paid for each incoming HTLC. This field combined with BaseFee is
	// used to compute the required fee for a given HTLC.
	//For asset , it is 1/10000
	FeeRate lnwire.UnitPrec11

	// TimeLockDelta is the absolute time-lock value, expressed in blocks,
	// that will be subtracted from an incoming HTLC's timelock value to
	// create the time-lock value for the forwarded outgoing HTLC. The
	// following constraint MUST hold for an HTLC to be forwarded:
	//
	//  * incomingHtlc.timeLock - timeLockDelta = fwdInfo.OutgoingCTLV
	//
	//    where fwdInfo is the forwarding information extracted from the
	//    per-hop payload of the incoming HTLC's onion packet.
	TimeLockDelta uint32

	// TODO(roasbeef): add fee module inside of switch
}

// ExpectedFee computes the expected fee for a given htlc amount. The value
// returned from this function is to be used as a sanity check when forwarding
// HTLC's to ensure that an incoming HTLC properly adheres to our propagated
// forwarding policy.
//
// TODO(roasbeef): also add in current available channel bandwidth, inverse
// func
func ExpectedFee(f ForwardingPolicy,
	htlcAmt lnwire.UnitPrec11) lnwire.UnitPrec11 {

	return lnwire.UnitPrec11(f.BaseFee) + (htlcAmt*f.FeeRate)/1000000
}

// ChannelLinkConfig defines the configuration for the channel link. ALL
// elements within the configuration MUST be non-nil for channel link to carry
// out its duties.
type ChannelLinkConfig struct {
	// FwrdingPolicy is the initial forwarding policy to be used when
	// deciding whether to forwarding incoming HTLC's or not. This value
	// can be updated with subsequent calls to UpdateForwardingPolicy
	// targeted at a given ChannelLink concrete interface implementation.
	FwrdingPolicy ForwardingPolicy

	// Circuits provides restricted access to the switch's circuit map,
	// allowing the link to open and close circuits.
	Circuits CircuitModifier

	// Switch provides a reference to the HTLC switch, we only use this in
	// testing to access circuit operations not typically exposed by the
	// CircuitModifier.
	//
	// TODO(conner): remove after refactoring htlcswitch testing framework.
	Switch *Switch

	// BestHeight returns the best known height.
	BestHeight func() uint32

	// ForwardPackets attempts to forward the batch of htlcs through the
	// switch. The function returns and error in case it fails to send one or
	// more packets. The link's quit signal should be provided to allow
	// cancellation of forwarding during link shutdown.
	ForwardPackets func(chan struct{}, ...*htlcPacket) error

	// DecodeHopIterators facilitates batched decoding of HTLC Sphinx onion
	// blobs, which are then used to inform how to forward an HTLC.
	//
	// NOTE: This function assumes the same set of readers and preimages
	// are always presented for the same identifier.
	DecodeHopIterators func([]byte, []hop.DecodeHopIteratorRequest) (
		[]hop.DecodeHopIteratorResponse, error)

	// ExtractErrorEncrypter function is responsible for decoding HTLC
	// Sphinx onion blob, and creating onion failure obfuscator.
	ExtractErrorEncrypter hop.ErrorEncrypterExtracter

	// FetchLastChannelUpdate retrieves the latest routing policy for a
	// target channel. This channel will typically be the outgoing channel
	// specified when we receive an incoming HTLC.  This will be used to
	// provide payment senders our latest policy when sending encrypted
	// error messages.
	FetchLastChannelUpdate func(lnwire.ShortChannelID) (*lnwire.ChannelUpdate, error)

	// Peer is a lightning network node with which we have the channel link
	// opened.
	Peer lnpeer.Peer

	// Registry is a sub-system which responsible for managing the invoices
	// in thread-safe manner.
	Registry InvoiceDatabase

	// PreimageCache is a global witness beacon that houses any new
	// preimages discovered by other links. We'll use this to add new
	// witnesses that we discover which will notify any sub-systems
	// subscribed to new events.
	PreimageCache contractcourt.WitnessBeacon

	// OnChannelFailure is a function closure that we'll call if the
	// channel failed for some reason. Depending on the severity of the
	// error, the closure potentially must force close this channel and
	// disconnect the peer.
	//
	// NOTE: The method must return in order for the ChannelLink to be able
	// to shut down properly.
	OnChannelFailure func(lnwire.ChannelID, lnwire.ShortChannelID,
		LinkFailureError)

	// UpdateContractSignals is a function closure that we'll use to update
	// outside sub-systems with the latest signals for our inner Lightning
	// channel. These signals will notify the caller when the channel has
	// been closed, or when the set of active HTLC's is updated.
	UpdateContractSignals func(*contractcourt.ContractSignals) error

	// ChainEvents is an active subscription to the chain watcher for this
	// channel to be notified of any on-chain activity related to this
	// channel.
	ChainEvents *contractcourt.ChainEventSubscription

	// FeeEstimator is an instance of a live fee estimator which will be
	// used to dynamically regulate the current fee of the commitment
	// transaction to ensure timely confirmation.
	FeeEstimator chainfee.Estimator

	// hodl.Mask is a bitvector composed of hodl.Flags, specifying breakpoints
	// for HTLC forwarding internal to the switch.
	//
	// NOTE: This should only be used for testing.
	HodlMask hodl.Mask

	// SyncStates is used to indicate that we need send the channel
	// reestablishment message to the remote peer. It should be done if our
	// clients have been restarted, or remote peer have been reconnected.
	SyncStates bool

	// BatchTicker is the ticker that determines the interval that we'll
	// use to check the batch to see if there're any updates we should
	// flush out. By batching updates into a single commit, we attempt to
	// increase throughput by maximizing the number of updates coalesced
	// into a single commit.
	BatchTicker ticker.Ticker

	// FwdPkgGCTicker is the ticker determining the frequency at which
	// garbage collection of forwarding packages occurs. We use a
	// time-based approach, as opposed to block epochs, as to not hinder
	// syncing.
	FwdPkgGCTicker ticker.Ticker

	// PendingCommitTicker is a ticker that allows the link to determine if
	// a locally initiated commitment dance gets stuck waiting for the
	// remote party to revoke.
	PendingCommitTicker ticker.Ticker

	// BatchSize is the max size of a batch of updates done to the link
	// before we do a state update.
	BatchSize uint32

	// UnsafeReplay will cause a link to replay the adds in its latest
	// commitment txn after the link is restarted. This should only be used
	// in testing, it is here to ensure the sphinx replay detection on the
	// receiving node is persistent.
	UnsafeReplay bool

	// MinFeeUpdateTimeout represents the minimum interval in which a link
	// will propose to update its commitment fee rate. A random timeout will
	// be selected between this and MaxFeeUpdateTimeout.
	MinFeeUpdateTimeout time.Duration

	// MaxFeeUpdateTimeout represents the maximum interval in which a link
	// will propose to update its commitment fee rate. A random timeout will
	// be selected between this and MinFeeUpdateTimeout.
	MaxFeeUpdateTimeout time.Duration

	// OutgoingCltvRejectDelta defines the number of blocks before expiry of
	// an htlc where we don't offer an htlc anymore. This should be at least
	// the outgoing broadcast delta, because in any case we don't want to
	// risk offering an htlc that triggers channel closure.
	OutgoingCltvRejectDelta uint32

	// TowerClient is an optional engine that manages the signing,
	// encrypting, and uploading of justice transactions to the daemon's
	// configured set of watchtowers for legacy channels.
	TowerClient TowerClient

	// MaxOutgoingCltvExpiry is the maximum outgoing timelock that the link
	// should accept for a forwarded HTLC. The value is relative to the
	// current block height.
	MaxOutgoingCltvExpiry uint32

	// MaxFeeAllocation is the highest allocation we'll allow a channel's
	// commitment fee to be of its balance. This only applies to the
	// initiator of the channel.
	MaxFeeAllocation float64

	// MaxAnchorsCommitFeeRate is the max commitment fee rate we'll use as
	// the initiator for channels of the anchor type.
	MaxAnchorsCommitFeeRate chainfee.SatPerKWeight

	// NotifyActiveLink allows the link to tell the ChannelNotifier when a
	// link is first started.
	NotifyActiveLink func(wire.OutPoint)

	// NotifyActiveChannel allows the link to tell the ChannelNotifier when
	// channels becomes active.
	NotifyActiveChannel func(wire.OutPoint)

	// NotifyInactiveChannel allows the switch to tell the ChannelNotifier
	// when channels become inactive.
	NotifyInactiveChannel func(wire.OutPoint)

	// HtlcNotifier is an instance of a htlcNotifier which we will pipe htlc
	// events through.
	HtlcNotifier htlcNotifier
}

// localUpdateAddMsg contains a locally initiated htlc and a channel that will
// receive the outcome of the link processing. This channel must be buffered to
// prevent the link from blocking.
type localUpdateAddMsg struct {
	pkt *htlcPacket
	err chan error
}

// shutdownReq contains an error channel that will be used by the channelLink
// to send an error if shutdown failed. If shutdown succeeded, the channel will
// be closed.
type shutdownReq struct {
	err chan error
}

//
//// channelLink is the service which drives a channel's commitment update
//// state-machine. In the event that an HTLC needs to be propagated to another
//// link, the forward handler from config is used which sends HTLC to the
//// switch. Additionally, the link encapsulate logic of commitment protocol
//// message ordering and updates.
//type channelLink struct {
//	// The following fields are only meant to be used *atomically*
//	started       int32
//	reestablished int32
//	shutdown      int32
//
//	// failed should be set to true in case a link error happens, making
//	// sure we don't process any more updates.
//	failed bool
//
//	// keystoneBatch represents a volatile list of keystones that must be
//	// written before attempting to sign the next commitment txn. These
//	// represent all the HTLC's forwarded to the link from the switch. Once
//	// we lock them into our outgoing commitment, then the circuit has a
//	// keystone, and is fully opened.
//	keystoneBatch []Keystone
//
//	// openedCircuits is the set of all payment circuits that will be open
//	// once we make our next commitment. After making the commitment we'll
//	// ACK all these from our mailbox to ensure that they don't get
//	// re-delivered if we reconnect.
//	openedCircuits []CircuitKey
//
//	// closedCircuits is the set of all payment circuits that will be
//	// closed once we make our next commitment. After taking the commitment
//	// we'll ACK all these to ensure that they don't get re-delivered if we
//	// reconnect.
//	closedCircuits []CircuitKey
//
//	// channel is a lightning network channel to which we apply htlc
//	// updates.
//	channel *lnwallet.LightningChannel
//
//	// shortChanID is the most up to date short channel ID for the link.
//	shortChanID lnwire.ShortChannelID
//
//	// cfg is a structure which carries all dependable fields/handlers
//	// which may affect behaviour of the service.
//	cfg ChannelLinkConfig
//
//	// mailBox is the main interface between the outside world and the
//	// link. All incoming messages will be sent over this mailBox. Messages
//	// include new updates from our connected peer, and new packets to be
//	// forwarded sent by the switch.
//	mailBox MailBox
//
//	// upstream is a channel that new messages sent from the remote peer to
//	// the local peer will be sent across.
//	upstream chan lnwire.Message
//
//	// downstream is a channel in which new multi-hop HTLC's to be
//	// forwarded will be sent across. Messages from this channel are sent
//	// by the HTLC switch.
//	downstream chan *htlcPacket
//
//	// localUpdateAdd is a channel to which locally initiated HTLCs are
//	// sent across.
//	localUpdateAdd chan *localUpdateAddMsg
//
//	// htlcUpdates is a channel that we'll use to update outside
//	// sub-systems with the latest set of active HTLC's on our channel.
//	htlcUpdates chan *contractcourt.ContractUpdate
//
//	// shutdownRequest is a channel that the channelLink will listen on to
//	// service shutdown requests from ShutdownIfChannelClean calls.
//	shutdownRequest chan *shutdownReq
//
//	// updateFeeTimer is the timer responsible for updating the link's
//	// commitment fee every time it fires.
//	updateFeeTimer *time.Timer
//
//	// uncommittedPreimages stores a list of all preimages that have been
//	// learned since receiving the last CommitSig from the remote peer. The
//	// batch will be flushed just before accepting the subsequent CommitSig
//	// or on shutdown to avoid doing a write for each preimage received.
//	uncommittedPreimages []lntypes.Preimage
//
//	sync.RWMutex
//
//	// hodlQueue is used to receive exit hop htlc resolutions from invoice
//	// registry.
//	hodlQueue *queue.ConcurrentQueue
//
//	// hodlMap stores related htlc data for a circuit key. It allows
//	// resolving those htlcs when we receive a message on hodlQueue.
//	hodlMap map[channeldb.CircuitKey]hodlHtlc
//
//	// log is a link-specific logging instance.
//	log btclog.Logger
//
//	wg   sync.WaitGroup
//	quit chan struct{}
//}

// hodlHtlc contains htlc data that is required for resolution.
type hodlHtlc struct {
	pd         *lnwallet.PaymentDescriptor
	obfuscator hop.ErrorEncrypter
}
