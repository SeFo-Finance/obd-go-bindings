package routing

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec"

	"github.com/SeFo-Finance/obd-go-bindings/channeldb"
	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
	"github.com/SeFo-Finance/obd-go-bindings/routing/route"
)

// BlockPadding is used to increment the finalCltvDelta value for the last hop
// to prevent an HTLC being failed if some blocks are mined while it's in-flight.
const BlockPadding uint16 = 3

// ValidateCLTVLimit is a helper function that validates that the cltv limit is
// greater than the final cltv delta parameter, optionally including the
// BlockPadding in this calculation.
func ValidateCLTVLimit(limit uint32, delta uint16, includePad bool) error {
	if includePad {
		delta += BlockPadding
	}

	if limit <= uint32(delta) {
		return fmt.Errorf("cltv limit %v should be greater than %v",
			limit, delta)
	}

	return nil
}

// noRouteError encodes a non-critical error encountered during path finding.
type noRouteError uint8

const (
	// errNoTlvPayload is returned when the destination hop does not support
	// a tlv payload.
	errNoTlvPayload noRouteError = iota

	// errNoPaymentAddr is returned when the destination hop does not
	// support payment addresses.
	errNoPaymentAddr

	// errNoPathFound is returned when a path to the target destination does
	// not exist in the graph.
	errNoPathFound

	// errInsufficientLocalBalance is returned when none of the local
	// channels have enough balance for the payment.
	errInsufficientBalance

	// errEmptyPaySession is returned when the empty payment session is
	// queried for a route.
	errEmptyPaySession

	// errUnknownRequiredFeature is returned when the destination node
	// requires an unknown feature.
	errUnknownRequiredFeature

	// errMissingDependentFeature is returned when the destination node
	// misses a feature that a feature that we require depends on.
	errMissingDependentFeature
)

var (
	// DefaultShardMinAmt is the default amount beyond which we won't try to
	// further split the payment if no route is found. It is the minimum
	// amount that we use as the shard size when splitting.
	DefaultShardMinAmt = lnwire.NewMSatFromSatoshis(10000)
)

// Error returns the string representation of the noRouteError
func (e noRouteError) Error() string {
	switch e {
	case errNoTlvPayload:
		return "destination hop doesn't understand new TLV payloads"

	case errNoPaymentAddr:
		return "destination hop doesn't understand payment addresses"

	case errNoPathFound:
		return "unable to find a path to destination"

	case errEmptyPaySession:
		return "empty payment session"

	case errInsufficientBalance:
		return "insufficient local balance"

	case errUnknownRequiredFeature:
		return "unknown required feature"

	case errMissingDependentFeature:
		return "missing dependent feature"

	default:
		return "unknown no-route error"
	}
}

// FailureReason converts a path finding error into a payment-level failure.
func (e noRouteError) FailureReason() channeldb.FailureReason {
	switch e {
	case
		errNoTlvPayload,
		errNoPaymentAddr,
		errNoPathFound,
		errEmptyPaySession,
		errUnknownRequiredFeature,
		errMissingDependentFeature:

		return channeldb.FailureReasonNoRoute

	case errInsufficientBalance:
		return channeldb.FailureReasonInsufficientBalance

	default:
		return channeldb.FailureReasonError
	}
}

// PaymentSession is used during SendPayment attempts to provide routes to
// attempt. It also defines methods to give the PaymentSession additional
// information learned during the previous attempts.
type PaymentSession interface {
	// RequestRoute returns the next route to attempt for routing the
	// specified HTLC payment to the target node. The returned route should
	// carry at most maxAmt to the target node, and pay at most feeLimit in
	// fees. It can carry less if the payment is MPP. The activeShards
	// argument should be set to instruct the payment session about the
	// number of in flight HTLCS for the payment, such that it can choose
	// splitting strategy accordingly.
	//
	// A noRouteError is returned if a non-critical error is encountered
	// during path finding.
	RequestRoute(maxAmt, feeLimit lnwire.UnitPrec11,
		activeShards, height uint32) (*route.Route, error)

	// UpdateAdditionalEdge takes an additional channel edge policy
	// (private channels) and applies the update from the message. Returns
	// a boolean to indicate whether the update has been applied without
	// error.
	UpdateAdditionalEdge(msg *lnwire.ChannelUpdate, pubKey *btcec.PublicKey,
		policy *channeldb.CachedEdgePolicy) bool

	// GetAdditionalEdgePolicy uses the public key and channel ID to query
	// the ephemeral channel edge policy for additional edges. Returns a nil
	// if nothing found.
	GetAdditionalEdgePolicy(pubKey *btcec.PublicKey,
		channelID uint64) *channeldb.CachedEdgePolicy
}

func getAmtValue(msat lnwire.MilliSatoshi, assetId uint32) uint64 {
	if assetId != lnwire.BtcAssetId { //asset
		return uint64(msat / 1000)
	}
	//btc
	return uint64(msat)
}
