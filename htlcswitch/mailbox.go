package htlcswitch

import (
	"errors"
	"time"

	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/lightningnetwork/lnd/lnwire"
)

var (
	// ErrMailBoxShuttingDown is returned when the mailbox is interrupted by
	// a shutdown request.
	ErrMailBoxShuttingDown = errors.New("mailbox is shutting down")

	// ErrPacketAlreadyExists signals that an attempt to add a packet failed
	// because it already exists in the mailbox.
	ErrPacketAlreadyExists = errors.New("mailbox already has packet")
)

// MailBox is an interface which represents a concurrent-safe, in-order
// delivery queue for messages from the network and also from the main switch.
// This struct servers as a buffer between incoming messages, and messages to
// the handled by the link. Each of the mutating methods within this interface
// should be implemented in a non-blocking manner.
type MailBox interface {
	// AddMessage appends a new message to the end of the message queue.
	AddMessage(msg lnwire.Message) error

	// AddPacket appends a new message to the end of the packet queue.
	AddPacket(pkt *htlcPacket) error

	// HasPacket queries the packets for a circuit key, this is used to drop
	// packets bound for the switch that already have a queued response.
	HasPacket(CircuitKey) bool

	// AckPacket removes a packet from the mailboxes in-memory replay
	// buffer. This will prevent a packet from being delivered after a link
	// restarts if the switch has remained online. The returned boolean
	// indicates whether or not a packet with the passed incoming circuit
	// key was removed.
	AckPacket(CircuitKey) bool

	// FailAdd fails an UpdateAddHTLC that exists within the mailbox,
	// removing it from the in-memory replay buffer. This will prevent the
	// packet from being delivered after the link restarts if the switch has
	// remained online. The generated LinkError will show an
	// OutgoingFailureDownstreamHtlcAdd FailureDetail.
	FailAdd(pkt *htlcPacket)

	// MessageOutBox returns a channel that any new messages ready for
	// delivery will be sent on.
	MessageOutBox() chan lnwire.Message

	// PacketOutBox returns a channel that any new packets ready for
	// delivery will be sent on.
	PacketOutBox() chan *htlcPacket

	// Clears any pending wire messages from the inbox.
	ResetMessages() error

	// Reset the packet head to point at the first element in the list.
	ResetPackets() error

	// SetDustClosure takes in a closure that is used to evaluate whether
	// mailbox HTLC's are dust.
	//SetDustClosure(isDust dustClosure)

	// SetFeeRate sets the feerate to be used when evaluating dust.
	SetFeeRate(feerate chainfee.SatPerKWeight)

	// DustPackets returns the dust sum for Adds in the mailbox for the
	// local and remote commitments.
	DustPackets() (lnwire.MilliSatoshi, lnwire.MilliSatoshi)

	// Start starts the mailbox and any goroutines it needs to operate
	// properly.
	Start()

	// Stop signals the mailbox and its goroutines for a graceful shutdown.
	Stop()
}

// courierType is an enum that reflects the distinct types of messages a
// MailBox can handle. Each type will be placed in an isolated mail box and
// will have a dedicated goroutine for delivering the messages.
type courierType uint8

const (
	// wireCourier is a type of courier that handles wire messages.
	wireCourier courierType = iota

	// pktCourier is a type of courier that handles htlc packets.
	pktCourier
)

// pktWithExpiry wraps an incoming packet and records the time at which it it
// should be canceled from the mailbox. This will be used to detect if it gets
// stuck in the mailbox and inform when to cancel back.
type pktWithExpiry struct {
	pkt    *htlcPacket
	expiry time.Time
}

func (p *pktWithExpiry) deadline(clock clock.Clock) <-chan time.Time {
	return clock.TickAfter(p.expiry.Sub(clock.Now()))
}
