package routing

import (
	"bytes"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/davecgh/go-spew/spew"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/lntypes"

	"github.com/SeFo-Finance/obd-go-bindings/record"

	"github.com/SeFo-Finance/obd-go-bindings/channeldb"
	"github.com/SeFo-Finance/obd-go-bindings/htlcswitch"
	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
	"github.com/SeFo-Finance/obd-go-bindings/routing/route"
	"github.com/SeFo-Finance/obd-go-bindings/zpay32"
)

const (
	// DefaultPayAttemptTimeout is the default payment attempt timeout. The
	// payment attempt timeout defines the duration after which we stop
	// trying more routes for a payment.
	DefaultPayAttemptTimeout = time.Duration(time.Second * 60)

	// DefaultChannelPruneExpiry is the default duration used to determine
	// if a channel should be pruned or not.
	DefaultChannelPruneExpiry = time.Duration(time.Hour * 24 * 14)

	// DefaultFirstTimePruneDelay is the time we'll wait after startup
	// before attempting to prune the graph for zombie channels. We don't
	// do it immediately after startup to allow lnd to start up without
	// getting blocked by this job.
	DefaultFirstTimePruneDelay = 30 * time.Second

	// defaultStatInterval governs how often the router will log non-empty
	// stats related to processing new channels, updates, or node
	// announcements.
	defaultStatInterval = time.Minute

	// MinCLTVDelta is the minimum CLTV value accepted by LND for all
	// timelock deltas. This includes both forwarding CLTV deltas set on
	// channel updates, as well as final CLTV deltas used to create BOLT 11
	// payment requests.
	//
	// NOTE: For payment requests, BOLT 11 stipulates that a final CLTV
	// delta of 9 should be used when no value is decoded. This however
	// leads to inflexiblity in upgrading this default parameter, since it
	// can create inconsistencies around the assumed value between sender
	// and receiver. Specifically, if the receiver assumes a higher value
	// than the sender, the receiver will always see the received HTLCs as
	// invalid due to their timelock not meeting the required delta.
	//
	// We skirt this by always setting an explicit CLTV delta when creating
	// invoices. This allows LND nodes to freely update the minimum without
	// creating incompatibilities during the upgrade process. For some time
	// LND has used an explicit default final CLTV delta of 40 blocks for
	// bitcoin (160 for litecoin), though we now clamp the lower end of this
	// range for user-chosen deltas to 18 blocks to be conservative.
	MinCLTVDelta = 18
)

var (
	// ErrRouterShuttingDown is returned if the router is in the process of
	// shutting down.
	ErrRouterShuttingDown = fmt.Errorf("router shutting down")
)

// PaymentAttemptDispatcher is used by the router to send payment attempts onto
// the network, and receive their results.
type PaymentAttemptDispatcher interface {
	// SendHTLC is a function that directs a link-layer switch to
	// forward a fully encoded payment to the first hop in the route
	// denoted by its public key. A non-nil error is to be returned if the
	// payment was unsuccessful.
	SendHTLC(firstHop lnwire.ShortChannelID,
		attemptID uint64,
		htlcAdd *lnwire.UpdateAddHTLC) error

	// GetPaymentResult returns the the result of the payment attempt with
	// the given attemptID. The paymentHash should be set to the payment's
	// overall hash, or in case of AMP payments the payment's unique
	// identifier.
	//
	// The method returns a channel where the payment result will be sent
	// when available, or an error is encountered during forwarding. When a
	// result is received on the channel, the HTLC is guaranteed to no
	// longer be in flight.  The switch shutting down is signaled by
	// closing the channel. If the attemptID is unknown,
	// ErrPaymentIDNotFound will be returned.
	GetPaymentResult(attemptID uint64, paymentHash lntypes.Hash,
		deobfuscator htlcswitch.ErrorDecrypter) (
		<-chan *htlcswitch.PaymentResult, error)

	// CleanStore calls the underlying result store, telling it is safe to
	// delete all entries except the ones in the keepPids map. This should
	// be called preiodically to let the switch clean up payment results
	// that we have handled.
	// NOTE: New payment attempts MUST NOT be made after the keepPids map
	// has been created and this method has returned.
	CleanStore(keepPids map[uint64]struct{}) error
}

// PaymentSessionSource is an interface that defines a source for the router to
// retrive new payment sessions.
type PaymentSessionSource interface {
	// NewPaymentSession creates a new payment session that will produce
	// routes to the given target. An optional set of routing hints can be
	// provided in order to populate additional edges to explore when
	// finding a path to the payment's destination.
	NewPaymentSession(p *LightningPayment) (PaymentSession, error)

	// NewPaymentSessionEmpty creates a new paymentSession instance that is
	// empty, and will be exhausted immediately. Used for failure reporting
	// to missioncontrol for resumed payment we don't want to make more
	// attempts for.
	NewPaymentSessionEmpty() PaymentSession
}

// MissionController is an interface that exposes failure reporting and
// probability estimation.
type MissionController interface {
	// ReportPaymentFail reports a failed payment to mission control as
	// input for future probability estimates. It returns a bool indicating
	// whether this error is a final error and no further payment attempts
	// need to be made.
	ReportPaymentFail(attemptID uint64, rt *route.Route,
		failureSourceIdx *int, failure lnwire.FailureMessage) (
		*channeldb.FailureReason, error)

	// ReportPaymentSuccess reports a successful payment to mission control as input
	// for future probability estimates.
	ReportPaymentSuccess(attemptID uint64, rt *route.Route) error

	// GetProbability is expected to return the success probability of a
	// payment from fromNode along edge.
	/*obd update wxf*/
	GetProbability(fromNode, toNode route.Vertex,
		amt lnwire.UnitPrec11) float64
}

// FeeSchema is the set fee configuration for a Lightning Node on the network.
// Using the coefficients described within the schema, the required fee to
// forward outgoing payments can be derived.
type FeeSchema struct {
	// BaseFee is the base amount of milli-satoshis that will be chained
	// for ANY payment forwarded.
	BaseFee lnwire.MilliSatoshi

	// FeeRate is the rate that will be charged for forwarding payments.
	// This value should be interpreted as the numerator for a fraction
	// (fixed point arithmetic) whose denominator is 1 million. As a result
	// the effective fee rate charged per mSAT will be: (amount *
	// FeeRate/1,000,000).
	FeeRate lnwire.UnitPrec11
}

// ChannelPolicy holds the parameters that determine the policy we enforce
// when forwarding payments on a channel. These parameters are communicated
// to the rest of the network in ChannelUpdate messages.
type ChannelPolicy struct {
	// FeeSchema holds the fee configuration for a channel.
	FeeSchema

	// TimeLockDelta is the required HTLC timelock delta to be used
	// when forwarding payments.
	TimeLockDelta uint32

	// MaxHTLC is the maximum HTLC size including fees we are allowed to
	// forward over this channel.
	MaxHTLC lnwire.UnitPrec11

	// MinHTLC is the minimum HTLC size including fees we are allowed to
	// forward over this channel.
	MinHTLC *lnwire.UnitPrec11
}

// EdgeLocator is a struct used to identify a specific edge.
type EdgeLocator struct {
	// ChannelID is the channel of this edge.
	ChannelID uint64

	// Direction takes the value of 0 or 1 and is identical in definition to
	// the channel direction flag. A value of 0 means the direction from the
	// lower node pubkey to the higher.
	Direction uint8
}

// String returns a human readable version of the edgeLocator values.
func (e *EdgeLocator) String() string {
	return fmt.Sprintf("%v:%v", e.ChannelID, e.Direction)
}

// generateNewSessionKey generates a new ephemeral private key to be used for a
// payment attempt.
func generateNewSessionKey() (*btcec.PrivateKey, error) {
	// Generate a new random session key to ensure that we don't trigger
	// any replay.
	//
	// TODO(roasbeef): add more sources of randomness?
	return btcec.NewPrivateKey(btcec.S256())
}

// generateSphinxPacket generates then encodes a sphinx packet which encodes
// the onion route specified by the passed layer 3 route. The blob returned
// from this function can immediately be included within an HTLC add packet to
// be sent to the first hop within the route.
func generateSphinxPacket(rt *route.Route, paymentHash []byte,
	sessionKey *btcec.PrivateKey) ([]byte, *sphinx.Circuit, error) {

	// Now that we know we have an actual route, we'll map the route into a
	// sphinx payument path which includes per-hop paylods for each hop
	// that give each node within the route the necessary information
	// (fees, CLTV value, etc) to properly forward the payment.
	sphinxPath, err := rt.ToSphinxPath()
	if err != nil {
		return nil, nil, err
	}

	log.Tracef("Constructed per-hop payloads for payment_hash=%x: %v",
		paymentHash[:], newLogClosure(func() string {
			path := make([]sphinx.OnionHop, sphinxPath.TrueRouteLength())
			for i := range path {
				hopCopy := sphinxPath[i]
				hopCopy.NodePub.Curve = nil
				path[i] = hopCopy
			}
			return spew.Sdump(path)
		}),
	)

	// Next generate the onion routing packet which allows us to perform
	// privacy preserving source routing across the network.
	sphinxPacket, err := sphinx.NewOnionPacket(
		sphinxPath, sessionKey, paymentHash,
		sphinx.DeterministicPacketFiller,
	)
	if err != nil {
		return nil, nil, err
	}

	// Finally, encode Sphinx packet using its wire representation to be
	// included within the HTLC add packet.
	var onionBlob bytes.Buffer
	if err := sphinxPacket.Encode(&onionBlob); err != nil {
		return nil, nil, err
	}

	log.Tracef("Generated sphinx packet: %v",
		newLogClosure(func() string {
			// We make a copy of the ephemeral key and unset the
			// internal curve here in order to keep the logs from
			// getting noisy.
			key := *sphinxPacket.EphemeralKey
			key.Curve = nil
			packetCopy := *sphinxPacket
			packetCopy.EphemeralKey = &key
			return spew.Sdump(packetCopy)
		}),
	)

	return onionBlob.Bytes(), &sphinx.Circuit{
		SessionKey:  sessionKey,
		PaymentPath: sphinxPath.NodeKeys(),
	}, nil
}

// LightningPayment describes a payment to be sent through the network to the
// final destination.
type LightningPayment struct {
	// Target is the node in which the payment should be routed towards.
	Target route.Vertex

	// Amount is the value of the payment to send through the network in
	// milli-satoshis.
	//Amount lnwire.MilliSatoshi

	/*obd update wxf*/
	Amount  lnwire.UnitPrec11
	AssetId uint32

	// FeeLimit is the maximum fee in millisatoshis that the payment should
	// accept when sending it through the network. The payment will fail
	// if there isn't a route with lower fees than this limit.
	//FeeLimit lnwire.MilliSatoshi

	/*obd update wxf*/
	FeeLimit lnwire.UnitPrec11

	// CltvLimit is the maximum time lock that is allowed for attempts to
	// complete this payment.
	CltvLimit uint32

	// paymentHash is the r-hash value to use within the HTLC extended to
	// the first hop. This won't be set for AMP payments.
	paymentHash *lntypes.Hash

	// amp is an optional field that is set if and only if this is am AMP
	// payment.
	amp *AMPOptions

	// FinalCLTVDelta is the CTLV expiry delta to use for the _final_ hop
	// in the route. This means that the final hop will have a CLTV delta
	// of at least: currentHeight + FinalCLTVDelta.
	FinalCLTVDelta uint16

	// PayAttemptTimeout is a timeout value that we'll use to determine
	// when we should should abandon the payment attempt after consecutive
	// payment failure. This prevents us from attempting to send a payment
	// indefinitely. A zero value means the payment will never time out.
	//
	// TODO(halseth): make wallclock time to allow resume after startup.
	PayAttemptTimeout time.Duration

	// RouteHints represents the different routing hints that can be used to
	// assist a payment in reaching its destination successfully. These
	// hints will act as intermediate hops along the route.
	//
	// NOTE: This is optional unless required by the payment. When providing
	// multiple routes, ensure the hop hints within each route are chained
	// together and sorted in forward order in order to reach the
	// destination successfully.
	RouteHints [][]zpay32.HopHint

	// OutgoingChannelIDs is the list of channels that are allowed for the
	// first hop. If nil, any channel may be used.
	OutgoingChannelIDs []uint64

	// LastHop is the pubkey of the last node before the final destination
	// is reached. If nil, any node may be used.
	LastHop *route.Vertex

	// DestFeatures specifies the set of features we assume the final node
	// has for pathfinding. Typically these will be taken directly from an
	// invoice, but they can also be manually supplied or assumed by the
	// sender. If a nil feature vector is provided, the router will try to
	// fallback to the graph in order to load a feature vector for a node in
	// the public graph.
	DestFeatures *lnwire.FeatureVector

	// PaymentAddr is the payment address specified by the receiver. This
	// field should be a random 32-byte nonce presented in the receiver's
	// invoice to prevent probing of the destination.
	PaymentAddr *[32]byte

	// PaymentRequest is an optional payment request that this payment is
	// attempting to complete.
	PaymentRequest []byte

	// DestCustomRecords are TLV records that are to be sent to the final
	// hop in the new onion payload format. If the destination does not
	// understand this new onion payload format, then the payment will
	// fail.
	DestCustomRecords record.CustomSet

	// MaxParts is the maximum number of partial payments that may be used
	// to complete the full amount.
	MaxParts uint32

	// MaxShardAmt is the largest shard that we'll attempt to split using.
	// If this field is set, and we need to split, rather than attempting
	// half of the original payment amount, we'll use this value if half
	// the payment amount is greater than it.
	//
	// NOTE: This field is _optional_.
	MaxShardAmt *lnwire.UnitPrec11
}

// AMPOptions houses information that must be known in order to send an AMP
// payment.
type AMPOptions struct {
	SetID     [32]byte
	RootShare [32]byte
}

// SetPaymentHash sets the given hash as the payment's overall hash. This
// should only be used for non-AMP payments.
func (l *LightningPayment) SetPaymentHash(hash lntypes.Hash) error {
	if l.amp != nil {
		return fmt.Errorf("cannot set payment hash for AMP payment")
	}

	l.paymentHash = &hash
	return nil
}

// SetAMP sets the given AMP options for the payment.
func (l *LightningPayment) SetAMP(amp *AMPOptions) error {
	if l.paymentHash != nil {
		return fmt.Errorf("cannot set amp options for payment " +
			"with payment hash")
	}

	l.amp = amp
	return nil
}

// Identifier returns a 32-byte slice that uniquely identifies this single
// payment. For non-AMP payments this will be the payment hash, for AMP
// payments this will be the used SetID.
func (l *LightningPayment) Identifier() [32]byte {
	if l.amp != nil {
		return l.amp.SetID
	}

	return *l.paymentHash
}
