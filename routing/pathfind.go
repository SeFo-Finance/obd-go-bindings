package routing

import (
	"math"

	"github.com/SeFo-Finance/obd-go-bindings/channeldb"
	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
	"github.com/SeFo-Finance/obd-go-bindings/record"
	"github.com/SeFo-Finance/obd-go-bindings/routing/route"
)

const (
	// infinity is used as a starting distance in our shortest path search.
	infinity = math.MaxInt64

	// RiskFactorBillionths controls the influence of time lock delta
	// of a channel on route selection. It is expressed as billionths
	// of msat per msat sent through the channel per time lock delta
	// block. See edgeWeight function below for more details.
	// The chosen value is based on the previous incorrect weight function
	// 1 + timelock + fee * fee. In this function, the fee penalty
	// diminishes the time lock penalty for all but the smallest amounts.
	// To not change the behaviour of path finding too drastically, a
	// relatively small value is chosen which is still big enough to give
	// some effect with smaller time lock values. The value may need
	// tweaking and/or be made configurable in the future.
	RiskFactorBillionths = 15

	// estimatedNodeCount is used to preallocate the path finding structures
	// to avoid resizing and copies. It should be number on the same order as
	// the number of active nodes in the network.
	estimatedNodeCount = 10000
)

var (
	// DefaultAttemptCost is the default fixed virtual cost in path finding
	// of a failed payment attempt. It is used to trade off potentially
	// better routes against their probability of succeeding.
	DefaultAttemptCost = lnwire.NewMSatFromSatoshis(100)

	// DefaultAttemptCostPPM is the default proportional virtual cost in
	// path finding weight units of executing a payment attempt that fails.
	// It is used to trade off potentially better routes against their
	// probability of succeeding. This parameter is expressed in parts per
	// million of the payment amount.
	//
	// It is impossible to pick a perfect default value. The current value
	// of 0.1% is based on the idea that a transaction fee of 1% is within
	// reasonable territory and that a payment shouldn't need more than 10
	// attempts.
	DefaultAttemptCostPPM = int64(1000)

	// DefaultMinRouteProbability is the default minimum probability for routes
	// returned from findPath.
	DefaultMinRouteProbability = float64(0.01)

	// DefaultAprioriHopProbability is the default a priori probability for
	// a hop.
	DefaultAprioriHopProbability = float64(0.6)
)

// edgePolicyWithSource is a helper struct to keep track of the source node
// of a channel edge. ChannelEdgePolicy only contains to destination node
// of the edge.
type edgePolicyWithSource struct {
	sourceNode route.Vertex
	edge       *channeldb.CachedEdgePolicy
}

// finalHopParams encapsulates various parameters for route construction that
// apply to the final hop in a route. These features include basic payment data
// such as amounts and cltvs, as well as more complex features like destination
// custom records and payment address.
type finalHopParams struct {
	/*obd update wxf
	AssetId >0 amt and totalAmt's unit is lnwire.MilliSatoshi, else omnicore.Amount
	*/
	amt         lnwire.UnitPrec11
	totalAmt    lnwire.UnitPrec11
	cltvDelta   uint16
	records     record.CustomSet
	paymentAddr *[32]byte
}

//
//// newRoute constructs a route using the provided path and final hop constraints.
//// Any destination specific fields from the final hop params  will be attached
//// assuming the destination's feature vector signals support, otherwise this
//// method will fail.  If the route is too long, or the selected path cannot
//// support the fully payment including fees, then a non-nil error is returned.
////
//// NOTE: The passed slice of ChannelHops MUST be sorted in forward order: from
//// the source to the target node of the path finding attempt. It is assumed that
//// any feature vectors on all hops have been validated for transitive
//// dependencies.
//func newRoute(sourceVertex route.Vertex,
//	pathEdges []*channeldb.CachedEdgePolicy, currentHeight uint32,
//	finalHop finalHopParams) (*route.Route, error) {
//
//	var (
//		hops []*route.Hop
//
//		// totalTimeLock will accumulate the cumulative time lock
//		// across the entire route. This value represents how long the
//		// sender will need to wait in the *worst* case.
//		totalTimeLock = currentHeight
//
//		// nextIncomingAmount is the amount that will need to flow into
//		// the *next* hop. Since we're going to be walking the route
//		// backwards below, this next hop gets closer and closer to the
//		// sender of the payment.
//		nextIncomingAmount lnwire.UnitPrec11
//	)
//
//	assetId := uint32(0)
//	pathLength := len(pathEdges)
//	for i := pathLength - 1; i >= 0; i-- {
//		// Now we'll start to calculate the items within the per-hop
//		// payload for the hop this edge is leading to.
//		edge := pathEdges[i]
//		assetId = edge.AssetId
//		// We'll calculate the amounts, timelocks, and fees for each hop
//		// in the route. The base case is the final hop which includes
//		// their amount and timelocks. These values will accumulate
//		// contributions from the preceding hops back to the sender as
//		// we compute the route in reverse.
//		var (
//			amtToForward     lnwire.UnitPrec11
//			fee              lnwire.UnitPrec11
//			outgoingTimeLock uint32
//			tlvPayload       bool
//			customRecords    record.CustomSet
//			mpp              *record.MPP
//		)
//
//		// Define a helper function that checks this edge's feature
//		// vector for support for a given feature. We assume at this
//		// point that the feature vectors transitive dependencies have
//		// been validated.
//		supports := func(feature lnwire.FeatureBit) bool {
//			// If this edge comes from router hints, the features
//			// could be nil.
//			if edge.ToNodeFeatures == nil {
//				return false
//			}
//			return edge.ToNodeFeatures.HasFeature(feature)
//		}
//
//		// We start by assuming the node doesn't support TLV. We'll now
//		// inspect the node's feature vector to see if we can promote
//		// the hop. We assume already that the feature vector's
//		// transitive dependencies have already been validated by path
//		// finding or some other means.
//		tlvPayload = supports(lnwire.TLVOnionPayloadOptional)
//
//		if i == len(pathEdges)-1 {
//			// If this is the last hop, then the hop payload will
//			// contain the exact amount. In BOLT #4: Onion Routing
//			// Protocol / "Payload for the Last Node", this is
//			// detailed.
//			amtToForward = finalHop.amt
//
//			// Fee is not part of the hop payload, but only used for
//			// reporting through RPC. Set to zero for the final hop.
//			fee = (0)
//
//			// As this is the last hop, we'll use the specified
//			// final CLTV delta value instead of the value from the
//			// last link in the route.
//			totalTimeLock += uint32(finalHop.cltvDelta)
//			outgoingTimeLock = totalTimeLock
//
//			// Attach any custom records to the final hop if the
//			// receiver supports TLV.
//			if !tlvPayload && finalHop.records != nil {
//				return nil, errors.New("cannot attach " +
//					"custom records")
//			}
//			customRecords = finalHop.records
//
//			// If we're attaching a payment addr but the receiver
//			// doesn't support both TLV and payment addrs, fail.
//			payAddr := supports(lnwire.PaymentAddrOptional)
//			if !payAddr && finalHop.paymentAddr != nil {
//				return nil, errors.New("cannot attach " +
//					"payment addr")
//			}
//
//			// Otherwise attach the mpp record if it exists.
//			// TODO(halseth): move this to payment life cycle,
//			// where AMP options are set.
//			if finalHop.paymentAddr != nil {
//				mpp = record.NewMPP(
//					finalHop.totalAmt,
//					*finalHop.paymentAddr,
//					edge.AssetId,
//				)
//			}
//		} else {
//			// The amount that the current hop needs to forward is
//			// equal to the incoming amount of the next hop.
//			amtToForward = nextIncomingAmount
//
//			// The fee that needs to be paid to the current hop is
//			// based on the amount that this hop needs to forward
//			// and its policy for the outgoing channel. This policy
//			// is stored as part of the incoming channel of
//			// the next hop.
//			fee = pathEdges[i+1].ComputeFee(amtToForward)
//
//			// We'll take the total timelock of the preceding hop as
//			// the outgoing timelock or this hop. Then we'll
//			// increment the total timelock incurred by this hop.
//			outgoingTimeLock = totalTimeLock
//			totalTimeLock += uint32(pathEdges[i+1].TimeLockDelta)
//		}
//
//		// Since we're traversing the path backwards atm, we prepend
//		// each new hop such that, the final slice of hops will be in
//		// the forwards order.
//		currentHop := &route.Hop{
//			PubKeyBytes:      edge.ToNodePubKey(),
//			ChannelID:        edge.ChannelID,
//			AmtToForward:     amtToForward,
//			OutgoingTimeLock: outgoingTimeLock,
//			LegacyPayload:    !tlvPayload,
//			CustomRecords:    customRecords,
//			MPP:              mpp,
//		}
//
//		hops = append([]*route.Hop{currentHop}, hops...)
//
//		// Finally, we update the amount that needs to flow into the
//		// *next* hop, which is the amount this hop needs to forward,
//		// accounting for the fee that it takes.
//		nextIncomingAmount = amtToForward + fee
//	}
//
//	// With the base routing data expressed as hops, build the full route
//	newRoute, err := route.NewRouteFromHops(assetId,
//		nextIncomingAmount, totalTimeLock, route.Vertex(sourceVertex),
//		hops,
//	)
//	if err != nil {
//		return nil, err
//	}
//
//	return newRoute, nil
//}
//
//// edgeWeight computes the weight of an edge. This value is used when searching
//// for the shortest path within the channel graph between two nodes. Weight is
//// is the fee itself plus a time lock penalty added to it. This benefits
//// channels with shorter time lock deltas and shorter (hops) routes in general.
//// RiskFactor controls the influence of time lock on route selection. This is
//// currently a fixed value, but might be configurable in the future.
////func edgeWeight(lockedAmt lnwire.MilliSatoshi, fee lnwire.MilliSatoshi,
////	timeLockDelta uint16) int64 {
///*obd update wxf*/
//func edgeWeight(lockedAmt, fee lnwire.UnitPrec11, timeLockDelta uint16) int64 {
//	// timeLockPenalty is the penalty for the time lock delta of this channel.
//	// It is controlled by RiskFactorBillionths and scales proportional
//	// to the amount that will pass through channel. Rationale is that it if
//	// a twice as large amount gets locked up, it is twice as bad.
//	timeLockPenalty := int64(lockedAmt) * int64(timeLockDelta) *
//		RiskFactorBillionths / 1000000000
//
//	return int64(fee) + timeLockPenalty
//}
//
//// RestrictParams wraps the set of restrictions passed to findPath that the
//// found path must adhere to.
//type RestrictParams struct {
//	// ProbabilitySource is a callback that is expected to return the
//	// success probability of traversing the channel from the node.
//	//ProbabilitySource func(route.Vertex, route.Vertex,
//	//	lnwire.MilliSatoshi) float64
//	/*obd update wxf*/
//	ProbabilitySource func(route.Vertex, route.Vertex,
//		lnwire.UnitPrec11) float64
//
//	// FeeLimit is a maximum fee amount allowed to be used on the path from
//	// the source to the target.
//	//FeeLimit lnwire.MilliSatoshi
//
//	/*obd update wxf*/
//	FeeLimit lnwire.UnitPrec11
//
//	// OutgoingChannelIDs is the list of channels that are allowed for the
//	// first hop. If nil, any channel may be used.
//	OutgoingChannelIDs []uint64
//
//	// LastHop is the pubkey of the last node before the final destination
//	// is reached. If nil, any node may be used.
//	LastHop *route.Vertex
//
//	// CltvLimit is the maximum time lock of the route excluding the final
//	// ctlv. After path finding is complete, the caller needs to increase
//	// all cltv expiry heights with the required final cltv delta.
//	CltvLimit uint32
//
//	// DestCustomRecords contains the custom records to drop off at the
//	// final hop, if any.
//	DestCustomRecords record.CustomSet
//
//	// DestFeatures is a feature vector describing what the final hop
//	// supports. If none are provided, pathfinding will try to inspect any
//	// features on the node announcement instead.
//	DestFeatures *lnwire.FeatureVector
//
//	// PaymentAddr is a random 32-byte value generated by the receiver to
//	// mitigate probing vectors and payment sniping attacks on overpaid
//	// invoices.
//	PaymentAddr *[32]byte
//}
//
//// PathFindingConfig defines global parameters that control the trade-off in
//// path finding between fees and probabiity.
//type PathFindingConfig struct {
//	// AttemptCost is the fixed virtual cost in path finding of a failed
//	// payment attempt. It is used to trade off potentially better routes
//	// against their probability of succeeding.
//	AttemptCost lnwire.MilliSatoshi
//
//	// AttemptCostPPM is the proportional virtual cost in path finding of a
//	// failed payment attempt. It is used to trade off potentially better
//	// routes against their probability of succeeding. This parameter is
//	// expressed in parts per million of the total payment amount.
//	AttemptCostPPM int64
//
//	// MinProbability defines the minimum success probability of the
//	// returned route.
//	MinProbability float64
//}
//
//// getProbabilityBasedDist converts a weight into a distance that takes into
//// account the success probability and the (virtual) cost of a failed payment
//// attempt.
////
//// Derivation:
////
//// Suppose there are two routes A and B with fees Fa and Fb and success
//// probabilities Pa and Pb.
////
//// Is the expected cost of trying route A first and then B lower than trying the
//// other way around?
////
//// The expected cost of A-then-B is: Pa*Fa + (1-Pa)*Pb*(c+Fb)
////
//// The expected cost of B-then-A is: Pb*Fb + (1-Pb)*Pa*(c+Fa)
////
//// In these equations, the term representing the case where both A and B fail is
//// left out because its value would be the same in both cases.
////
//// Pa*Fa + (1-Pa)*Pb*(c+Fb) < Pb*Fb + (1-Pb)*Pa*(c+Fa)
////
//// Pa*Fa + Pb*c + Pb*Fb - Pa*Pb*c - Pa*Pb*Fb < Pb*Fb + Pa*c + Pa*Fa - Pa*Pb*c - Pa*Pb*Fa
////
//// Removing terms that cancel out:
//// Pb*c - Pa*Pb*Fb < Pa*c - Pa*Pb*Fa
////
//// Divide by Pa*Pb:
//// c/Pa - Fb < c/Pb - Fa
////
//// Move terms around:
//// Fa + c/Pa < Fb + c/Pb
////
//// So the value of F + c/P can be used to compare routes.
//func getProbabilityBasedDist(weight int64, probability float64, penalty int64) int64 {
//	// Clamp probability to prevent overflow.
//	const minProbability = 0.00001
//
//	if probability < minProbability {
//		return infinity
//	}
//
//	return weight + int64(float64(penalty)/probability)
//}
