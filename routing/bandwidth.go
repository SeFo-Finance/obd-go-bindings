package routing

import (
	"github.com/SeFo-Finance/obd-go-bindings/htlcswitch"

	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
)

// bandwidthHints provides hints about the currently available balance in our
// channels.
type bandwidthHints interface {
	// availableChanBandwidth returns the total available bandwidth for a
	// channel and a bool indicating whether the channel hint was found.
	// The amount parameter is used to validate the outgoing htlc amount
	// that we wish to add to the channel against its flow restrictions. If
	// a zero amount is provided, the minimum htlc value for the channel
	// will be used. If the channel is unavailable, a zero amount is
	// returned.
	availableChanBandwidth(channelID uint64,
		amount lnwire.UnitPrec11) (lnwire.UnitPrec11, bool)
}

// getLinkQuery is the function signature used to lookup a link.
type getLinkQuery func(lnwire.ShortChannelID) (
	htlcswitch.ChannelLink, error)
