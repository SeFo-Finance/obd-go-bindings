package routing

import (
	"sync"

	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/multimutex"
	"github.com/lightningnetwork/lnd/queue"

	"github.com/SeFo-Finance/obd-go-bindings/channeldb"
)

// ControlTower tracks all outgoing payments made, whose primary purpose is to
// prevent duplicate payments to the same payment hash. In production, a
// persistent implementation is preferred so that tracking can survive across
// restarts. Payments are transitioned through various payment states, and the
// ControlTower interface provides access to driving the state transitions.
type ControlTower interface {
	// This method checks that no suceeded payment exist for this payment
	// hash.
	InitPayment(lntypes.Hash, *channeldb.PaymentCreationInfo) error

	// RegisterAttempt atomically records the provided HTLCAttemptInfo.
	RegisterAttempt(lntypes.Hash, *channeldb.HTLCAttemptInfo) error

	// SettleAttempt marks the given attempt settled with the preimage. If
	// this is a multi shard payment, this might implicitly mean the the
	// full payment succeeded.
	//
	// After invoking this method, InitPayment should always return an
	// error to prevent us from making duplicate payments to the same
	// payment hash. The provided preimage is atomically saved to the DB
	// for record keeping.
	SettleAttempt(lntypes.Hash, uint64, *channeldb.HTLCSettleInfo) (
		*channeldb.HTLCAttempt, error)

	// FailAttempt marks the given payment attempt failed.
	FailAttempt(lntypes.Hash, uint64, *channeldb.HTLCFailInfo) (
		*channeldb.HTLCAttempt, error)

	// FetchPayment fetches the payment corresponding to the given payment
	// hash.
	FetchPayment(paymentHash lntypes.Hash) (*channeldb.MPPayment, error)

	// Fail transitions a payment into the Failed state, and records the
	// ultimate reason the payment failed. Note that this should only be
	// called when all active active attempts are already failed. After
	// invoking this method, InitPayment should return nil on its next call
	// for this payment hash, allowing the user to make a subsequent
	// payment.
	Fail(lntypes.Hash, channeldb.FailureReason) error

	// FetchInFlightPayments returns all payments with status InFlight.
	FetchInFlightPayments() ([]*channeldb.MPPayment, error)

	// SubscribePayment subscribes to updates for the payment with the given
	// hash. A first update with the current state of the payment is always
	// sent out immediately.
	SubscribePayment(paymentHash lntypes.Hash) (*ControlTowerSubscriber,
		error)
}

// ControlTowerSubscriber contains the state for a payment update subscriber.
type ControlTowerSubscriber struct {
	// Updates is the channel over which *channeldb.MPPayment updates can be
	// received.
	Updates <-chan interface{}

	queue *queue.ConcurrentQueue
	quit  chan struct{}
}

// newControlTowerSubscriber instantiates a new subscriber state object.
func newControlTowerSubscriber() *ControlTowerSubscriber {
	// Create a queue for payment updates.
	queue := queue.NewConcurrentQueue(20)
	queue.Start()

	return &ControlTowerSubscriber{
		Updates: queue.ChanOut(),
		queue:   queue,
		quit:    make(chan struct{}),
	}
}

// Close signals that the subscriber is no longer interested in updates.
func (s *ControlTowerSubscriber) Close() {
	// Close quit channel so that any pending writes to the queue are
	// cancelled.
	close(s.quit)

	// Stop the queue goroutine so that it won't leak.
	s.queue.Stop()
}

// controlTower is persistent implementation of ControlTower to restrict
// double payment sending.
type controlTower struct {
	db *channeldb.PaymentControl

	subscribers    map[lntypes.Hash][]*ControlTowerSubscriber
	subscribersMtx sync.Mutex

	// paymentsMtx provides synchronization on the payment level to ensure
	// that no race conditions occur in between updating the database and
	// sending a notification.
	paymentsMtx *multimutex.HashMutex
}
