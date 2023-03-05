package invoicesrpc

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/lightningnetwork/lnd/lntypes"

	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
	"github.com/SeFo-Finance/obd-go-bindings/zpay32"
)

const (
	// DefaultInvoiceExpiry is the default invoice expiry for new MPP
	// invoices.
	DefaultInvoiceExpiry = 24 * time.Hour

	// DefaultAMPInvoiceExpiry is the default invoice expiry for new AMP
	// invoices.
	DefaultAMPInvoiceExpiry = 30 * 24 * time.Hour
)

// AddInvoiceData contains the required data to create a new invoice.
type AddInvoiceData struct {
	// An optional memo to attach along with the invoice. Used for record
	// keeping purposes for the invoice's creator, and will also be set in
	// the description field of the encoded payment request if the
	// description_hash field is not being used.
	Memo string

	// The preimage which will allow settling an incoming HTLC payable to
	// this preimage. If Preimage is set, Hash should be nil. If both
	// Preimage and Hash are nil, a random preimage is generated.
	Preimage *lntypes.Preimage

	// The hash of the preimage. If Hash is set, Preimage should be nil.
	// This condition indicates that we have a 'hold invoice' for which the
	// htlc will be accepted and held until the preimage becomes known.
	Hash *lntypes.Hash

	// The value of this invoice in millisatoshis.
	Value   lnwire.UnitPrec11
	AssetId uint32

	// Hash (SHA-256) of a description of the payment. Used if the
	// description of payment (memo) is too long to naturally fit within the
	// description field of an encoded payment request.
	DescriptionHash []byte

	// Payment request expiry time in seconds. Default is 3600 (1 hour).
	Expiry int64

	// Fallback on-chain address.
	FallbackAddr string

	// Delta to use for the time-lock of the CLTV extended to the final hop.
	CltvExpiry uint64

	// Whether this invoice should include routing hints for private
	// channels.
	Private bool

	// HodlInvoice signals that this invoice shouldn't be settled
	// immediately upon receiving the payment.
	HodlInvoice bool

	// Amp signals whether or not to create an AMP invoice.
	//
	// NOTE: Preimage should always be set to nil when this value is true.
	Amp bool

	// RouteHints are optional route hints that can each be individually used
	// to assist in reaching the invoice's destination.
	RouteHints [][]zpay32.HopHint
}

// paymentHashAndPreimage returns the payment hash and preimage for this invoice
// depending on the configuration.
//
// For AMP invoices (when Amp flag is true), this method always returns a nil
// preimage. The hash value can be set externally by the user using the Hash
// field, or one will be generated randomly. The payment hash here only serves
// as a unique identifier for insertion into the invoice index, as there is
// no universal preimage for an AMP payment.
//
// For MPP invoices (when Amp flag is false), this method may return nil
// preimage when create a hodl invoice, but otherwise will always return a
// non-nil preimage and the corresponding payment hash. The valid combinations
// are parsed as follows:
//   - Preimage == nil && Hash == nil -> (random preimage, H(random preimage))
//   - Preimage != nil && Hash == nil -> (Preimage, H(Preimage))
//   - Preimage == nil && Hash != nil -> (nil, Hash)
func (d *AddInvoiceData) paymentHashAndPreimage() (
	*lntypes.Preimage, lntypes.Hash, error) {

	if d.Amp {
		return d.ampPaymentHashAndPreimage()
	}

	return d.mppPaymentHashAndPreimage()
}

// ampPaymentHashAndPreimage returns the payment hash to use for an AMP invoice.
// The preimage will always be nil.
func (d *AddInvoiceData) ampPaymentHashAndPreimage() (*lntypes.Preimage, lntypes.Hash, error) {
	switch {

	// Preimages cannot be set on AMP invoice.
	case d.Preimage != nil:
		return nil, lntypes.Hash{},
			errors.New("preimage set on AMP invoice")

	// If a specific hash was requested, use that.
	case d.Hash != nil:
		return nil, *d.Hash, nil

	// Otherwise generate a random hash value, just needs to be unique to be
	// added to the invoice index.
	default:
		var paymentHash lntypes.Hash
		if _, err := rand.Read(paymentHash[:]); err != nil {
			return nil, lntypes.Hash{}, err
		}

		return nil, paymentHash, nil
	}
}

// mppPaymentHashAndPreimage returns the payment hash and preimage to use for an
// MPP invoice.
func (d *AddInvoiceData) mppPaymentHashAndPreimage() (*lntypes.Preimage, lntypes.Hash, error) {
	var (
		paymentPreimage *lntypes.Preimage
		paymentHash     lntypes.Hash
	)

	switch {

	// Only either preimage or hash can be set.
	case d.Preimage != nil && d.Hash != nil:
		return nil, lntypes.Hash{},
			errors.New("preimage and hash both set")

	// If no hash or preimage is given, generate a random preimage.
	case d.Preimage == nil && d.Hash == nil:
		paymentPreimage = &lntypes.Preimage{}
		if _, err := rand.Read(paymentPreimage[:]); err != nil {
			return nil, lntypes.Hash{}, err
		}
		paymentHash = paymentPreimage.Hash()

	// If just a hash is given, we create a hold invoice by setting the
	// preimage to unknown.
	case d.Preimage == nil && d.Hash != nil:
		paymentHash = *d.Hash

	// A specific preimage was supplied. Use that for the invoice.
	case d.Preimage != nil && d.Hash == nil:
		preimage := *d.Preimage
		paymentPreimage = &preimage
		paymentHash = d.Preimage.Hash()
	}

	return paymentPreimage, paymentHash, nil
}
