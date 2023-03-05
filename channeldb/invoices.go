package channeldb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/tlv"

	"github.com/SeFo-Finance/obd-go-bindings/record"

	"github.com/SeFo-Finance/obd-go-bindings/feature"
	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
)

var (
	// unknownPreimage is an all-zeroes preimage that indicates that the
	// preimage for this invoice is not yet known.
	unknownPreimage lntypes.Preimage

	// BlankPayAddr is a sentinel payment address for legacy invoices.
	// Invoices with this payment address are special-cased in the insertion
	// logic to prevent being indexed in the payment address index,
	// otherwise they would cause collisions after the first insertion.
	BlankPayAddr [32]byte

	// invoiceBucket is the name of the bucket within the database that
	// stores all data related to invoices no matter their final state.
	// Within the invoice bucket, each invoice is keyed by its invoice ID
	// which is a monotonically increasing uint32.
	invoiceBucket = []byte("invoices")

	// paymentHashIndexBucket is the name of the sub-bucket within the
	// invoiceBucket which indexes all invoices by their payment hash. The
	// payment hash is the sha256 of the invoice's payment preimage. This
	// index is used to detect duplicates, and also to provide a fast path
	// for looking up incoming HTLCs to determine if we're able to settle
	// them fully.
	//
	// maps: payHash => invoiceKey
	invoiceIndexBucket = []byte("paymenthashes")

	// payAddrIndexBucket is the name of the top-level bucket that maps
	// payment addresses to their invoice number. This can be used
	// to efficiently query or update non-legacy invoices. Note that legacy
	// invoices will not be included in this index since they all have the
	// same, all-zero payment address, however all newly generated invoices
	// will end up in this index.
	//
	// maps: payAddr => invoiceKey
	payAddrIndexBucket = []byte("pay-addr-index")

	// setIDIndexBucket is the name of the top-level bucket that maps set
	// ids to their invoice number. This can be used to efficiently query or
	// update AMP invoice. Note that legacy or MPP invoices will not be
	// included in this index, since their HTLCs do not have a set id.
	//
	// maps: setID => invoiceKey
	setIDIndexBucket = []byte("set-id-index")

	// numInvoicesKey is the name of key which houses the auto-incrementing
	// invoice ID which is essentially used as a primary key. With each
	// invoice inserted, the primary key is incremented by one. This key is
	// stored within the invoiceIndexBucket. Within the invoiceBucket
	// invoices are uniquely identified by the invoice ID.
	numInvoicesKey = []byte("nik")

	// addIndexBucket is an index bucket that we'll use to create a
	// monotonically increasing set of add indexes. Each time we add a new
	// invoice, this sequence number will be incremented and then populated
	// within the new invoice.
	//
	// In addition to this sequence number, we map:
	//
	//   addIndexNo => invoiceKey
	addIndexBucket = []byte("invoice-add-index")

	// settleIndexBucket is an index bucket that we'll use to create a
	// monotonically increasing integer for tracking a "settle index". Each
	// time an invoice is settled, this sequence number will be incremented
	// as populate within the newly settled invoice.
	//
	// In addition to this sequence number, we map:
	//
	//   settleIndexNo => invoiceKey
	settleIndexBucket = []byte("invoice-settle-index")

	// ErrInvoiceAlreadySettled is returned when the invoice is already
	// settled.
	ErrInvoiceAlreadySettled = errors.New("invoice already settled")

	// ErrInvoiceAlreadyCanceled is returned when the invoice is already
	// canceled.
	ErrInvoiceAlreadyCanceled = errors.New("invoice already canceled")

	// ErrInvoiceAlreadyAccepted is returned when the invoice is already
	// accepted.
	ErrInvoiceAlreadyAccepted = errors.New("invoice already accepted")

	// ErrInvoiceStillOpen is returned when the invoice is still open.
	ErrInvoiceStillOpen = errors.New("invoice still open")

	// ErrInvoiceCannotOpen is returned when an attempt is made to move an
	// invoice to the open state.
	ErrInvoiceCannotOpen = errors.New("cannot move invoice to open")

	// ErrInvoiceCannotAccept is returned when an attempt is made to accept
	// an invoice while the invoice is not in the open state.
	ErrInvoiceCannotAccept = errors.New("cannot accept invoice")

	// ErrInvoicePreimageMismatch is returned when the preimage doesn't
	// match the invoice hash.
	ErrInvoicePreimageMismatch = errors.New("preimage does not match")

	// ErrHTLCPreimageMissing is returned when trying to accept/settle an
	// AMP HTLC but the HTLC-level preimage has not been set.
	ErrHTLCPreimageMissing = errors.New("AMP htlc missing preimage")

	// ErrHTLCPreimageMismatch is returned when trying to accept/settle an
	// AMP HTLC but the HTLC-level preimage does not satisfying the
	// HTLC-level payment hash.
	ErrHTLCPreimageMismatch = errors.New("htlc preimage mismatch")

	// ErrHTLCAlreadySettled is returned when trying to settle an invoice
	// but HTLC already exists in the settled state.
	ErrHTLCAlreadySettled = errors.New("htlc already settled")

	// ErrInvoiceHasHtlcs is returned when attempting to insert an invoice
	// that already has HTLCs.
	ErrInvoiceHasHtlcs = errors.New("cannot add invoice with htlcs")

	// ErrEmptyHTLCSet is returned when attempting to accept or settle and
	// HTLC set that has no HTLCs.
	ErrEmptyHTLCSet = errors.New("cannot settle/accept empty HTLC set")

	// ErrUnexpectedInvoicePreimage is returned when an invoice-level
	// preimage is provided when trying to settle an invoice that shouldn't
	// have one, e.g. an AMP invoice.
	ErrUnexpectedInvoicePreimage = errors.New(
		"unexpected invoice preimage provided on settle",
	)

	// ErrHTLCPreimageAlreadyExists is returned when trying to set an
	// htlc-level preimage but one is already known.
	ErrHTLCPreimageAlreadyExists = errors.New(
		"htlc-level preimage already exists",
	)
)

// ErrDuplicateSetID is an error returned when attempting to adding an AMP HTLC
// to an invoice, but another invoice is already indexed by the same set id.
type ErrDuplicateSetID struct {
	setID [32]byte
}

// Error returns a human-readable description of ErrDuplicateSetID.
func (e ErrDuplicateSetID) Error() string {
	return fmt.Sprintf("invoice with set_id=%x already exists", e.setID)
}

const (
	// MaxMemoSize is maximum size of the memo field within invoices stored
	// in the database.
	MaxMemoSize = 1024

	// MaxPaymentRequestSize is the max size of a payment request for
	// this invoice.
	// TODO(halseth): determine the max length payment request when field
	// lengths are final.
	MaxPaymentRequestSize = 4096

	// A set of tlv type definitions used to serialize invoice htlcs to the
	// database.
	//
	// NOTE: A migration should be added whenever this list changes. This
	// prevents against the database being rolled back to an older
	// format where the surrounding logic might assume a different set of
	// fields are known.
	chanIDType       tlv.Type = 1
	htlcIDType       tlv.Type = 3
	amtType          tlv.Type = 5
	acceptHeightType tlv.Type = 7
	acceptTimeType   tlv.Type = 9
	resolveTimeType  tlv.Type = 11
	expiryHeightType tlv.Type = 13
	htlcStateType    tlv.Type = 15
	mppTotalAmtType  tlv.Type = 17
	htlcAMPType      tlv.Type = 19
	htlcHashType     tlv.Type = 21
	htlcPreimageType tlv.Type = 23
	/*obd add wxf*/
	assetIdType tlv.Type = 25

	// A set of tlv type definitions used to serialize invoice bodiees.
	//
	// NOTE: A migration should be added whenever this list changes. This
	// prevents against the database being rolled back to an older
	// format where the surrounding logic might assume a different set of
	// fields are known.
	memoType            tlv.Type = 0
	payReqType          tlv.Type = 1
	createTimeType      tlv.Type = 2
	settleTimeType      tlv.Type = 3
	addIndexType        tlv.Type = 4
	settleIndexType     tlv.Type = 5
	preimageType        tlv.Type = 6
	valueType           tlv.Type = 7
	cltvDeltaType       tlv.Type = 8
	expiryType          tlv.Type = 9
	paymentAddrType     tlv.Type = 10
	featuresType        tlv.Type = 11
	invStateType        tlv.Type = 12
	amtPaidType         tlv.Type = 13
	hodlInvoiceType     tlv.Type = 14
	invoiceAmpStateType tlv.Type = 15

	// A set of tlv type definitions used to serialize the invoice AMP
	// state along-side the main invoice body.
	ampStateSetIDType       tlv.Type = 0
	ampStateHtlcStateType   tlv.Type = 1
	ampStateSettleIndexType tlv.Type = 2
	ampStateSettleDateType  tlv.Type = 3
	ampStateCircuitKeysType tlv.Type = 4
	ampStateAmtPaidType     tlv.Type = 5
	ampStateAssetIdType     tlv.Type = 6
)

// RefModifier is a modification on top of a base invoice ref. It allows the
// caller to opt to skip out on HTLCs for a given payAddr, or only return the
// set of specified HTLCs for a given setID.
type RefModifier uint8

const (
	// DefaultModifier is the base modifier that doesn't change any behavior.
	DefaultModifier RefModifier = iota

	// HtlcSetOnlyModifier can only be used with a setID based invoice ref, and
	// specifies that only the set of HTLCs related to that setID are to be
	// returned.
	HtlcSetOnlyModifier

	// HtlcSetOnlyModifier can only be used with a payAddr based invoice ref,
	// and specifies that the returned invoice shouldn't include any HTLCs at
	// all.
	HtlcSetBlankModifier
)

// InvoiceRef is a composite identifier for invoices. Invoices can be referenced
// by various combinations of payment hash and payment addr, in certain contexts
// only some of these are known. An InvoiceRef and its constructors thus
// encapsulate the valid combinations of query parameters that can be supplied
// to LookupInvoice and UpdateInvoice.
type InvoiceRef struct {
	// payHash is the payment hash of the target invoice. All invoices are
	// currently indexed by payment hash. This value will be used as a
	// fallback when no payment address is known.
	payHash *lntypes.Hash

	// payAddr is the payment addr of the target invoice. Newer invoices
	// (0.11 and up) are indexed by payment address in addition to payment
	// hash, but pre 0.8 invoices do not have one at all. When this value is
	// known it will be used as the primary identifier, falling back to
	// payHash if no value is known.
	payAddr *[32]byte

	// setID is the optional set id for an AMP payment. This can be used to
	// lookup or update the invoice knowing only this value. Queries by set
	// id are only used to facilitate user-facing requests, e.g. lookup,
	// settle or cancel an AMP invoice. The regular update flow from the
	// invoice registry will always query for the invoice by
	// payHash+payAddr.
	setID *[32]byte

	// refModifier allows an invoice ref to include or exclude specific
	// HTLC sets based on the payAddr or setId.
	refModifier RefModifier
}

// InvoiceRefByHash creates an InvoiceRef that queries for an invoice only by
// its payment hash.
func InvoiceRefByHash(payHash lntypes.Hash) InvoiceRef {
	return InvoiceRef{
		payHash: &payHash,
	}
}

// InvoiceRefByHashAndAddr creates an InvoiceRef that first queries for an
// invoice by the provided payment address, falling back to the payment hash if
// the payment address is unknown.
func InvoiceRefByHashAndAddr(payHash lntypes.Hash,
	payAddr [32]byte) InvoiceRef {

	return InvoiceRef{
		payHash: &payHash,
		payAddr: &payAddr,
	}
}

// InvoiceRefByAddr creates an InvoiceRef that queries the payment addr index
// for an invoice with the provided payment address.
func InvoiceRefByAddr(addr [32]byte) InvoiceRef {
	return InvoiceRef{
		payAddr: &addr,
	}
}

// InvoiceRefByAddrBlankHtlc creates an InvoiceRef that queries the payment addr index
// for an invoice with the provided payment address, but excludes any of the
// core HTLC information.
func InvoiceRefByAddrBlankHtlc(addr [32]byte) InvoiceRef {
	return InvoiceRef{
		payAddr:     &addr,
		refModifier: HtlcSetBlankModifier,
	}
}

// InvoiceRefBySetID creates an InvoiceRef that queries the set id index for an
// invoice with the provided setID. If the invoice is not found, the query will
// not fallback to payHash or payAddr.
func InvoiceRefBySetID(setID [32]byte) InvoiceRef {
	return InvoiceRef{
		setID: &setID,
	}
}

// InvoiceRefBySetIDFiltered is similar to the InvoiceRefBySetID identifier,
// but it specifies that the returned set of HTLCs should be filtered to only
// include HTLCs that are part of that set.
func InvoiceRefBySetIDFiltered(setID [32]byte) InvoiceRef {
	return InvoiceRef{
		setID:       &setID,
		refModifier: HtlcSetOnlyModifier,
	}
}

// PayHash returns the optional payment hash of the target invoice.
//
// NOTE: This value may be nil.
func (r InvoiceRef) PayHash() *lntypes.Hash {
	if r.payHash != nil {
		hash := *r.payHash
		return &hash
	}
	return nil
}

// PayAddr returns the optional payment address of the target invoice.
//
// NOTE: This value may be nil.
func (r InvoiceRef) PayAddr() *[32]byte {
	if r.payAddr != nil {
		addr := *r.payAddr
		return &addr
	}
	return nil
}

// SetID returns the optional set id of the target invoice.
//
// NOTE: This value may be nil.
func (r InvoiceRef) SetID() *[32]byte {
	if r.setID != nil {
		id := *r.setID
		return &id
	}
	return nil
}

// Modifier defines the set of available modifications to the base invoice ref
// look up that are available.
func (r InvoiceRef) Modifier() RefModifier {
	return r.refModifier
}

// String returns a human-readable representation of an InvoiceRef.
func (r InvoiceRef) String() string {
	var ids []string
	if r.payHash != nil {
		ids = append(ids, fmt.Sprintf("pay_hash=%v", *r.payHash))
	}
	if r.payAddr != nil {
		ids = append(ids, fmt.Sprintf("pay_addr=%x", *r.payAddr))
	}
	if r.setID != nil {
		ids = append(ids, fmt.Sprintf("set_id=%x", *r.setID))
	}
	return fmt.Sprintf("(%s)", strings.Join(ids, ", "))
}

// ContractState describes the state the invoice is in.
type ContractState uint8

const (
	// ContractOpen means the invoice has only been created.
	ContractOpen ContractState = 0

	// ContractSettled means the htlc is settled and the invoice has been paid.
	ContractSettled ContractState = 1

	// ContractCanceled means the invoice has been canceled.
	ContractCanceled ContractState = 2

	// ContractAccepted means the HTLC has been accepted but not settled yet.
	ContractAccepted ContractState = 3
)

// String returns a human readable identifier for the ContractState type.
func (c ContractState) String() string {
	switch c {
	case ContractOpen:
		return "Open"
	case ContractSettled:
		return "Settled"
	case ContractCanceled:
		return "Canceled"
	case ContractAccepted:
		return "Accepted"
	}

	return "Unknown"
}

// IsFinal returns a boolean indicating whether an invoice state is final
func (c ContractState) IsFinal() bool {
	return c == ContractSettled || c == ContractCanceled
}

// ContractTerm is a companion struct to the Invoice struct. This struct houses
// the necessary conditions required before the invoice can be considered fully
// settled by the payee.
type ContractTerm struct {
	// FinalCltvDelta is the minimum required number of blocks before htlc
	// expiry when the invoice is accepted.
	FinalCltvDelta int32

	// Expiry defines how long after creation this invoice should expire.
	Expiry time.Duration

	// PaymentPreimage is the preimage which is to be revealed in the
	// occasion that an HTLC paying to the hash of this preimage is
	// extended. Set to nil if the preimage isn't known yet.
	PaymentPreimage *lntypes.Preimage

	// Value is the expected amount of milli-satoshis to be paid to an HTLC
	// which can be satisfied by the above preimage.
	Value lnwire.UnitPrec11
	/*obd update wxf*/
	AssetId uint32

	// PaymentAddr is a randomly generated value include in the MPP record
	// by the sender to prevent probing of the receiver.
	PaymentAddr [32]byte

	// Features is the feature vectors advertised on the payment request.
	Features *lnwire.FeatureVector
}

// String returns a human-readable description of the prominent contract terms.
func (c ContractTerm) String() string {
	return fmt.Sprintf("amt=%v, expiry=%v, final_cltv_delta=%v", c.Value,
		c.Expiry, c.FinalCltvDelta)
}

// SetID is the extra unique tuple item for AMP invoices. In addition to
// setting a payment address, each repeated payment to an AMP invoice will also
// contain a set ID as well.
type SetID [32]byte

// InvoiceStateAMP is a struct that associates the current state of an AMP
// invoice identified by its set ID along with the set of invoices identified
// by the circuit key. This allows callers to easily look up the latest state
// of an AMP "sub-invoice" and also look up the invoice HLTCs themselves in the
// greater HTLC map index.
type InvoiceStateAMP struct {
	// State is the state of this sub-AMP invoice.
	State HtlcState

	// SettleIndex indicates the location in the settle index that
	// references this instance of InvoiceStateAMP, but only if
	// this value is set (non-zero), and State is HtlcStateSettled.
	SettleIndex uint64

	// SettleDate is the date that the setID was settled.
	SettleDate time.Time

	// InvoiceKeys is the set of circuit keys that can be used to locate
	// the invoices for a given set ID.
	InvoiceKeys map[CircuitKey]struct{}

	// AmtPaid is the total amount that was paid in the AMP sub-invoice.
	// Fetching the full HTLC/invoice state allows one to extract the
	// custom records as well as the break down of the payment splits used
	// when paying.
	//AmtPaid lnwire.MilliSatoshi
	AmtPaid lnwire.UnitPrec11
	AssetId uint32
}

// AMPInvoiceState represents a type that stores metadata related to the set of
// settled AMP "sub-invoices".
type AMPInvoiceState map[SetID]InvoiceStateAMP

// recordSize returns the amount of bytes this TLV record will occupy when
// encoded.
func (a *AMPInvoiceState) recordSize() uint64 {
	var (
		b   bytes.Buffer
		buf [8]byte
	)

	// We know that encoding works since the tests pass in the build this file
	// is checked into, so we'll simplify things and simply encode it ourselves
	// then report the total amount of bytes used.
	if err := ampStateEncoder(&b, a, &buf); err != nil {
		// This should never error out, but we log it just in case it
		// does.
		log.Errorf("encoding the amp invoice state failed: %v", err)
	}

	return uint64(len(b.Bytes()))
}

// Invoice is a payment invoice generated by a payee in order to request
// payment for some good or service. The inclusion of invoices within Lightning
// creates a payment work flow for merchants very similar to that of the
// existing financial system within PayPal, etc.  Invoices are added to the
// database when a payment is requested, then can be settled manually once the
// payment is received at the upper layer. For record keeping purposes,
// invoices are never deleted from the database, instead a bit is toggled
// denoting the invoice has been fully settled. Within the database, all
// invoices must have a unique payment hash which is generated by taking the
// sha256 of the payment preimage.
type Invoice struct {
	// Memo is an optional memo to be stored along side an invoice.  The
	// memo may contain further details pertaining to the invoice itself,
	// or any other message which fits within the size constraints.
	Memo []byte

	// PaymentRequest is the encoded payment request for this invoice. For
	// spontaneous (keysend) payments, this field will be empty.
	PaymentRequest []byte

	// CreationDate is the exact time the invoice was created.
	CreationDate time.Time

	// SettleDate is the exact time the invoice was settled.
	SettleDate time.Time

	// Terms are the contractual payment terms of the invoice. Once all the
	// terms have been satisfied by the payer, then the invoice can be
	// considered fully fulfilled.
	//
	// TODO(roasbeef): later allow for multiple terms to fulfill the final
	// invoice: payment fragmentation, etc.
	Terms ContractTerm

	// AddIndex is an auto-incrementing integer that acts as a
	// monotonically increasing sequence number for all invoices created.
	// Clients can then use this field as a "checkpoint" of sorts when
	// implementing a streaming RPC to notify consumers of instances where
	// an invoice has been added before they re-connected.
	//
	// NOTE: This index starts at 1.
	AddIndex uint64

	// SettleIndex is an auto-incrementing integer that acts as a
	// monotonically increasing sequence number for all settled invoices.
	// Clients can then use this field as a "checkpoint" of sorts when
	// implementing a streaming RPC to notify consumers of instances where
	// an invoice has been settled before they re-connected.
	//
	// NOTE: This index starts at 1.
	SettleIndex uint64

	// State describes the state the invoice is in. This is the global
	// state of the invoice which may remain open even when a series of
	// sub-invoices for this invoice has been settled.
	State ContractState

	// AmtPaid is the final amount that we ultimately accepted for pay for
	// this invoice. We specify this value independently as it's possible
	// that the invoice originally didn't specify an amount, or the sender
	// overpaid.
	//AmtPaid lnwire.MilliSatoshi
	AmtPaid lnwire.UnitPrec11
	AssetId uint32

	// Htlcs records all htlcs that paid to this invoice. Some of these
	// htlcs may have been marked as canceled.
	Htlcs map[CircuitKey]*InvoiceHTLC

	// AMPState describes the state of any related sub-invoices AMP to this
	// greater invoice. A sub-invoice is defined by a set of HTLCs with the
	// same set ID that attempt to make one time or recurring payments to
	// this greater invoice. It's possible for a sub-invoice to be canceled
	// or settled, but the greater invoice still open.
	AMPState AMPInvoiceState

	// HodlInvoice indicates whether the invoice should be held in the
	// Accepted state or be settled right away.
	HodlInvoice bool
}

// HTLCSet returns the set of HTLCs belonging to setID and in the provided
// state. Passing a nil setID will return all HTLCs in the provided state in the
// case of legacy or MPP, and no HTLCs in the case of AMP.  Otherwise, the
// returned set will be filtered by the populated setID which is used to
// retrieve AMP HTLC sets.
func (i *Invoice) HTLCSet(setID *[32]byte, state HtlcState) map[CircuitKey]*InvoiceHTLC {
	htlcSet := make(map[CircuitKey]*InvoiceHTLC)
	for key, htlc := range i.Htlcs {
		// Only add HTLCs that are in the requested HtlcState.
		if htlc.State != state {
			continue
		}

		if !htlc.IsInHTLCSet(setID) {
			continue
		}

		htlcSet[key] = htlc
	}

	return htlcSet
}

// HTLCSetCompliment returns the set of all HTLCs not belonging to setID that
// are in the target state. Passing a nil setID will return no invoices, since
// all MPP HTLCs are part of the same HTLC set.
func (i *Invoice) HTLCSetCompliment(setID *[32]byte,
	state HtlcState) map[CircuitKey]*InvoiceHTLC {

	htlcSet := make(map[CircuitKey]*InvoiceHTLC)
	for key, htlc := range i.Htlcs {
		// Only add HTLCs that are in the requested HtlcState.
		if htlc.State != state {
			continue
		}

		// We are constructing the compliment, so filter anything that
		// matches this set id.
		if htlc.IsInHTLCSet(setID) {
			continue
		}

		htlcSet[key] = htlc
	}

	return htlcSet
}

// HtlcState defines the states an htlc paying to an invoice can be in.
type HtlcState uint8

const (
	// HtlcStateAccepted indicates the htlc is locked-in, but not resolved.
	HtlcStateAccepted HtlcState = iota

	// HtlcStateCanceled indicates the htlc is canceled back to the
	// sender.
	HtlcStateCanceled

	// HtlcStateSettled indicates the htlc is settled.
	HtlcStateSettled
)

// InvoiceHTLC contains details about an htlc paying to this invoice.
type InvoiceHTLC struct {
	// Amt is the amount that is carried by this htlc.
	//Amt lnwire.MilliSatoshi
	Amt     lnwire.UnitPrec11
	AssetId uint32

	// MppTotalAmt is a field for mpp that indicates the expected total
	// amount.
	//MppTotalAmt lnwire.MilliSatoshi
	MppTotalAmt lnwire.UnitPrec11

	// AcceptHeight is the block height at which the invoice registry
	// decided to accept this htlc as a payment to the invoice. At this
	// height, the invoice cltv delay must have been met.
	AcceptHeight uint32

	// AcceptTime is the wall clock time at which the invoice registry
	// decided to accept the htlc.
	AcceptTime time.Time

	// ResolveTime is the wall clock time at which the invoice registry
	// decided to settle the htlc.
	ResolveTime time.Time

	// Expiry is the expiry height of this htlc.
	Expiry uint32

	// State indicates the state the invoice htlc is currently in. A
	// canceled htlc isn't just removed from the invoice htlcs map, because
	// we need AcceptHeight to properly cancel the htlc back.
	State HtlcState

	// CustomRecords contains the custom key/value pairs that accompanied
	// the htlc.
	CustomRecords record.CustomSet

	// AMP encapsulates additional data relevant to AMP HTLCs. This includes
	// the AMP onion record, in addition to the HTLC's payment hash and
	// preimage since these are unique to each AMP HTLC, and not the invoice
	// as a whole.
	//
	// NOTE: This value will only be set for AMP HTLCs.
	AMP *InvoiceHtlcAMPData
}

// Copy makes a deep copy of the target InvoiceHTLC.
func (h *InvoiceHTLC) Copy() *InvoiceHTLC {
	result := *h

	// Make a copy of the CustomSet map.
	result.CustomRecords = make(record.CustomSet)
	for k, v := range h.CustomRecords {
		result.CustomRecords[k] = v
	}

	result.AMP = h.AMP.Copy()

	return &result
}

// IsInHTLCSet returns true if this HTLC is part an HTLC set. If nil is passed,
// this method returns true if this is an MPP HTLC. Otherwise, it only returns
// true if the AMP HTLC's set id matches the populated setID.
func (h *InvoiceHTLC) IsInHTLCSet(setID *[32]byte) bool {
	wantAMPSet := setID != nil
	isAMPHtlc := h.AMP != nil

	// Non-AMP HTLCs cannot be part of AMP HTLC sets, and vice versa.
	if wantAMPSet != isAMPHtlc {
		return false
	}

	// Skip AMP HTLCs that have differing set ids.
	if isAMPHtlc && *setID != h.AMP.Record.SetID() {
		return false
	}

	return true
}

// InvoiceHtlcAMPData is a struct hodling the additional metadata stored for
// each received AMP HTLC. This includes the AMP onion record, in addition to
// the HTLC's payment hash and preimage.
type InvoiceHtlcAMPData struct {
	// AMP is a copy of the AMP record presented in the onion payload
	// containing the information necessary to correlate and settle a
	// spontaneous HTLC set. Newly accepted legacy keysend payments will
	// also have this field set as we automatically promote them into an AMP
	// payment for internal processing.
	Record record.AMP

	// Hash is an HTLC-level payment hash that is stored only for AMP
	// payments. This is done because an AMP HTLC will carry a different
	// payment hash from the invoice it might be satisfying, so we track the
	// payment hashes individually to able to compute whether or not the
	// reconstructed preimage correctly matches the HTLC's hash.
	Hash lntypes.Hash

	// Preimage is an HTLC-level preimage that satisfies the AMP HTLC's
	// Hash. The preimage will be be derived either from secret share
	// reconstruction of the shares in the AMP payload.
	//
	// NOTE: Preimage will only be present once the HTLC is in
	// HtlcStateSettled.
	Preimage *lntypes.Preimage
}

// Copy returns a deep copy of the InvoiceHtlcAMPData.
func (d *InvoiceHtlcAMPData) Copy() *InvoiceHtlcAMPData {
	if d == nil {
		return nil
	}

	var preimage *lntypes.Preimage
	if d.Preimage != nil {
		pimg := *d.Preimage
		preimage = &pimg
	}

	return &InvoiceHtlcAMPData{
		Record:   d.Record,
		Hash:     d.Hash,
		Preimage: preimage,
	}
}

// HtlcAcceptDesc describes the details of a newly accepted htlc.
type HtlcAcceptDesc struct {
	// AcceptHeight is the block height at which this htlc was accepted.
	AcceptHeight int32

	// Amt is the amount that is carried by this htlc.
	//Amt lnwire.MilliSatoshi
	Amt     lnwire.UnitPrec11
	AssetId uint32

	// MppTotalAmt is a field for mpp that indicates the expected total
	// amount.
	//MppTotalAmt lnwire.MilliSatoshi
	MppTotalAmt lnwire.UnitPrec11

	// Expiry is the expiry height of this htlc.
	Expiry uint32

	// CustomRecords contains the custom key/value pairs that accompanied
	// the htlc.
	CustomRecords record.CustomSet

	// AMP encapsulates additional data relevant to AMP HTLCs. This includes
	// the AMP onion record, in addition to the HTLC's payment hash and
	// preimage since these are unique to each AMP HTLC, and not the invoice
	// as a whole.
	//
	// NOTE: This value will only be set for AMP HTLCs.
	AMP *InvoiceHtlcAMPData
}

// InvoiceUpdateDesc describes the changes that should be applied to the
// invoice.
type InvoiceUpdateDesc struct {
	// State is the new state that this invoice should progress to. If nil,
	// the state is left unchanged.
	State *InvoiceStateUpdateDesc

	// CancelHtlcs describes the htlcs that need to be canceled.
	CancelHtlcs map[CircuitKey]struct{}

	// AddHtlcs describes the newly accepted htlcs that need to be added to
	// the invoice.
	AddHtlcs map[CircuitKey]*HtlcAcceptDesc

	// SetID is an optional set ID for AMP invoices that allows operations
	// to be more efficient by ensuring we don't need to read out the
	// entire HTLC set each timee an HTLC is to be cancelled.
	SetID *SetID
}

// InvoiceStateUpdateDesc describes an invoice-level state transition.
type InvoiceStateUpdateDesc struct {
	// NewState is the new state that this invoice should progress to.
	NewState ContractState

	// Preimage must be set to the preimage when NewState is settled.
	Preimage *lntypes.Preimage

	// HTLCPreimages set the HTLC-level preimages stored for AMP HTLCs.
	// These are only learned when settling the invoice as a whole. Must be
	// set when settling an invoice with non-nil SetID.
	HTLCPreimages map[CircuitKey]lntypes.Preimage

	// SetID identifies a specific set of HTLCs destined for the same
	// invoice as part of a larger AMP payment. This value will be nil for
	// legacy or MPP payments.
	SetID *[32]byte
}

// InvoiceUpdateCallback is a callback used in the db transaction to update the
// invoice.
type InvoiceUpdateCallback = func(invoice *Invoice) (*InvoiceUpdateDesc, error)

func validateInvoice(i *Invoice, paymentHash lntypes.Hash) error {
	// Avoid conflicts with all-zeroes magic value in the database.
	if paymentHash == unknownPreimage.Hash() {
		return fmt.Errorf("cannot use hash of all-zeroes preimage")
	}

	if len(i.Memo) > MaxMemoSize {
		return fmt.Errorf("max length a memo is %v, and invoice "+
			"of length %v was provided", MaxMemoSize, len(i.Memo))
	}
	if len(i.PaymentRequest) > MaxPaymentRequestSize {
		return fmt.Errorf("max length of payment request is %v, length "+
			"provided was %v", MaxPaymentRequestSize,
			len(i.PaymentRequest))
	}
	if i.Terms.Features == nil {
		return errors.New("invoice must have a feature vector")
	}

	err := feature.ValidateDeps(i.Terms.Features)
	if err != nil {
		return err
	}

	// AMP invoices and hodl invoices are allowed to have no preimage
	// specified.
	isAMP := i.Terms.Features.HasFeature(
		lnwire.AMPOptional,
	)
	if i.Terms.PaymentPreimage == nil && !(i.HodlInvoice || isAMP) {
		return errors.New("non-hodl invoices must have a preimage")
	}

	if len(i.Htlcs) > 0 {
		return ErrInvoiceHasHtlcs
	}

	return nil
}

// IsPending returns ture if the invoice is in ContractOpen state.
func (i *Invoice) IsPending() bool {
	return i.State == ContractOpen || i.State == ContractAccepted
}

// fetchInvoiceNumByRef retrieve the invoice number for the provided invoice
// reference. The payment address will be treated as the primary key, falling
// back to the payment hash if nothing is found for the payment address. An
// error is returned if the invoice is not found.
func fetchInvoiceNumByRef(invoiceIndex, payAddrIndex, setIDIndex kvdb.RBucket,
	ref InvoiceRef) ([]byte, error) {

	// If the set id is present, we only consult the set id index for this
	// invoice. This type of query is only used to facilitate user-facing
	// requests to lookup, settle or cancel an AMP invoice.
	setID := ref.SetID()
	if setID != nil {
		invoiceNumBySetID := setIDIndex.Get(setID[:])
		if invoiceNumBySetID == nil {
			return nil, ErrInvoiceNotFound
		}

		return invoiceNumBySetID, nil
	}

	payHash := ref.PayHash()
	payAddr := ref.PayAddr()

	getInvoiceNumByHash := func() []byte {
		if payHash != nil {
			return invoiceIndex.Get(payHash[:])
		}
		return nil
	}

	getInvoiceNumByAddr := func() []byte {
		if payAddr != nil {
			// Only allow lookups for payment address if it is not a
			// blank payment address, which is a special-cased value
			// for legacy keysend invoices.
			if *payAddr != BlankPayAddr {
				return payAddrIndex.Get(payAddr[:])
			}
		}
		return nil
	}

	invoiceNumByHash := getInvoiceNumByHash()
	invoiceNumByAddr := getInvoiceNumByAddr()
	switch {

	// If payment address and payment hash both reference an existing
	// invoice, ensure they reference the _same_ invoice.
	case invoiceNumByAddr != nil && invoiceNumByHash != nil:
		if !bytes.Equal(invoiceNumByAddr, invoiceNumByHash) {
			return nil, ErrInvRefEquivocation
		}

		return invoiceNumByAddr, nil

	// Return invoices by payment addr only.
	//
	// NOTE: We constrain this lookup to only apply if the invoice ref does
	// not contain a payment hash. Legacy and MPP payments depend on the
	// payment hash index to enforce that the HTLCs payment hash matches the
	// payment hash for the invoice, without this check we would
	// inadvertently assume the invoice contains the correct preimage for
	// the HTLC, which we only enforce via the lookup by the invoice index.
	case invoiceNumByAddr != nil && payHash == nil:
		return invoiceNumByAddr, nil

	// If we were only able to reference the invoice by hash, return the
	// corresponding invoice number. This can happen when no payment address
	// was provided, or if it didn't match anything in our records.
	case invoiceNumByHash != nil:
		return invoiceNumByHash, nil

	// Otherwise we don't know of the target invoice.
	default:
		return nil, ErrInvoiceNotFound
	}
}

// InvoiceQuery represents a query to the invoice database. The query allows a
// caller to retrieve all invoices starting from a particular add index and
// limit the number of results returned.
type InvoiceQuery struct {
	// IndexOffset is the offset within the add indices to start at. This
	// can be used to start the response at a particular invoice.
	IndexOffset uint64

	// NumMaxInvoices is the maximum number of invoices that should be
	// starting from the add index.
	NumMaxInvoices uint64

	// PendingOnly, if set, returns unsettled invoices starting from the
	// add index.
	PendingOnly bool

	// Reversed, if set, indicates that the invoices returned should start
	// from the IndexOffset and go backwards.
	Reversed bool
	/* obd update wxf
	 *query invoice for special asset,set it to true.
	 */
	IsQueryAsset bool
	AssetId      uint32
	StartTime    int64
	EndTime      int64
}

// InvoiceSlice is the response to a invoice query. It includes the original
// query, the set of invoices that match the query, and an integer which
// represents the offset index of the last item in the set of returned invoices.
// This integer allows callers to resume their query using this offset in the
// event that the query's response exceeds the maximum number of returnable
// invoices.
type InvoiceSlice struct {
	InvoiceQuery

	// Invoices is the set of invoices that matched the query above.
	Invoices []Invoice

	// FirstIndexOffset is the index of the first element in the set of
	// returned Invoices above. Callers can use this to resume their query
	// in the event that the slice has too many events to fit into a single
	// response.
	FirstIndexOffset uint64

	// LastIndexOffset is the index of the last element in the set of
	// returned Invoices above. Callers can use this to resume their query
	// in the event that the slice has too many events to fit into a single
	// response.
	LastIndexOffset uint64
}

func putInvoice(invoices, invoiceIndex, payAddrIndex, addIndex kvdb.RwBucket,
	i *Invoice, invoiceNum uint32, paymentHash lntypes.Hash) (
	uint64, error) {

	// Create the invoice key which is just the big-endian representation
	// of the invoice number.
	var invoiceKey [4]byte
	byteOrder.PutUint32(invoiceKey[:], invoiceNum)

	// Increment the num invoice counter index so the next invoice bares
	// the proper ID.
	var scratch [4]byte
	invoiceCounter := invoiceNum + 1
	byteOrder.PutUint32(scratch[:], invoiceCounter)
	if err := invoiceIndex.Put(numInvoicesKey, scratch[:]); err != nil {
		return 0, err
	}

	// Add the payment hash to the invoice index. This will let us quickly
	// identify if we can settle an incoming payment, and also to possibly
	// allow a single invoice to have multiple payment installations.
	err := invoiceIndex.Put(paymentHash[:], invoiceKey[:])
	if err != nil {
		return 0, err
	}

	// Add the invoice to the payment address index, but only if the invoice
	// has a non-zero payment address. The all-zero payment address is still
	// in use by legacy keysend, so we special-case here to avoid
	// collisions.
	if i.Terms.PaymentAddr != BlankPayAddr {
		err = payAddrIndex.Put(i.Terms.PaymentAddr[:], invoiceKey[:])
		if err != nil {
			return 0, err
		}
	}

	// Next, we'll obtain the next add invoice index (sequence
	// number), so we can properly place this invoice within this
	// event stream.
	nextAddSeqNo, err := addIndex.NextSequence()
	if err != nil {
		return 0, err
	}

	// With the next sequence obtained, we'll updating the event series in
	// the add index bucket to map this current add counter to the index of
	// this new invoice.
	var seqNoBytes [8]byte
	byteOrder.PutUint64(seqNoBytes[:], nextAddSeqNo)
	if err := addIndex.Put(seqNoBytes[:], invoiceKey[:]); err != nil {
		return 0, err
	}

	i.AddIndex = nextAddSeqNo

	// Finally, serialize the invoice itself to be written to the disk.
	var buf bytes.Buffer
	if err := serializeInvoice(&buf, i); err != nil {
		return 0, err
	}

	if err := invoices.Put(invoiceKey[:], buf.Bytes()); err != nil {
		return 0, err
	}

	return nextAddSeqNo, nil
}

// serializeInvoice serializes an invoice to a writer.
//
// Note: this function is in use for a migration. Before making changes that
// would modify the on disk format, make a copy of the original code and store
// it with the migration.
func serializeInvoice(w io.Writer, i *Invoice) error {
	creationDateBytes, err := i.CreationDate.MarshalBinary()
	if err != nil {
		return err
	}

	settleDateBytes, err := i.SettleDate.MarshalBinary()
	if err != nil {
		return err
	}

	var fb bytes.Buffer
	err = i.Terms.Features.EncodeBase256(&fb)
	if err != nil {
		return err
	}
	featureBytes := fb.Bytes()

	preimage := [32]byte(unknownPreimage)
	if i.Terms.PaymentPreimage != nil {
		preimage = *i.Terms.PaymentPreimage
		if preimage == unknownPreimage {
			return errors.New("cannot use all-zeroes preimage")
		}
	}
	value := uint64(i.Terms.Value)
	cltvDelta := uint32(i.Terms.FinalCltvDelta)
	expiry := uint64(i.Terms.Expiry)

	amtPaid := uint64(i.AmtPaid)
	assetId := i.AssetId
	state := uint8(i.State)

	var hodlInvoice uint8
	if i.HodlInvoice {
		hodlInvoice = 1
	}

	tlvStream, err := tlv.NewStream(
		// Memo and payreq.
		tlv.MakePrimitiveRecord(memoType, &i.Memo),
		tlv.MakePrimitiveRecord(payReqType, &i.PaymentRequest),

		// Add/settle metadata.
		tlv.MakePrimitiveRecord(createTimeType, &creationDateBytes),
		tlv.MakePrimitiveRecord(settleTimeType, &settleDateBytes),
		tlv.MakePrimitiveRecord(addIndexType, &i.AddIndex),
		tlv.MakePrimitiveRecord(settleIndexType, &i.SettleIndex),

		// Terms.
		tlv.MakePrimitiveRecord(preimageType, &preimage),
		tlv.MakePrimitiveRecord(valueType, &value),
		tlv.MakePrimitiveRecord(cltvDeltaType, &cltvDelta),
		tlv.MakePrimitiveRecord(expiryType, &expiry),
		tlv.MakePrimitiveRecord(paymentAddrType, &i.Terms.PaymentAddr),
		tlv.MakePrimitiveRecord(featuresType, &featureBytes),

		// Invoice state.
		tlv.MakePrimitiveRecord(invStateType, &state),
		tlv.MakePrimitiveRecord(amtPaidType, &amtPaid),

		tlv.MakePrimitiveRecord(hodlInvoiceType, &hodlInvoice),

		// Invoice AMP state.
		tlv.MakeDynamicRecord(
			invoiceAmpStateType, &i.AMPState,
			i.AMPState.recordSize,
			ampStateEncoder, ampStateDecoder,
		),
		tlv.MakePrimitiveRecord(assetIdType, &assetId),
	)
	if err != nil {
		return err
	}

	var b bytes.Buffer
	if err = tlvStream.Encode(&b); err != nil {
		return err
	}

	err = binary.Write(w, byteOrder, uint64(b.Len()))
	if err != nil {
		return err
	}

	if _, err = w.Write(b.Bytes()); err != nil {
		return err
	}

	// Only if this is a _non_ AMP invoice do we serialize the HTLCs
	// in-line with the rest of the invoice.
	ampInvoice := i.Terms.Features.HasFeature(
		lnwire.AMPOptional,
	)
	if ampInvoice {
		return nil
	}

	return serializeHtlcs(w, i.Htlcs)
}

// serializeHtlcs serializes a map containing circuit keys and invoice htlcs to
// a writer.
func serializeHtlcs(w io.Writer, htlcs map[CircuitKey]*InvoiceHTLC) error {
	for key, htlc := range htlcs {
		// Encode the htlc in a tlv stream.
		chanID := key.ChanID.ToUint64()
		amt := uint64(htlc.Amt)
		assetId := uint64(htlc.AssetId)
		mppTotalAmt := uint64(htlc.MppTotalAmt)
		acceptTime := putNanoTime(htlc.AcceptTime)
		resolveTime := putNanoTime(htlc.ResolveTime)
		state := uint8(htlc.State)

		var records []tlv.Record
		records = append(records,
			tlv.MakePrimitiveRecord(chanIDType, &chanID),
			tlv.MakePrimitiveRecord(htlcIDType, &key.HtlcID),
			tlv.MakePrimitiveRecord(amtType, &amt),
			tlv.MakePrimitiveRecord(
				acceptHeightType, &htlc.AcceptHeight,
			),
			tlv.MakePrimitiveRecord(acceptTimeType, &acceptTime),
			tlv.MakePrimitiveRecord(resolveTimeType, &resolveTime),
			tlv.MakePrimitiveRecord(expiryHeightType, &htlc.Expiry),
			tlv.MakePrimitiveRecord(htlcStateType, &state),
			tlv.MakePrimitiveRecord(mppTotalAmtType, &mppTotalAmt),
		)

		if htlc.AMP != nil {
			setIDRecord := tlv.MakeDynamicRecord(
				htlcAMPType, &htlc.AMP.Record,
				htlc.AMP.Record.PayloadSize,
				record.AMPEncoder, record.AMPDecoder,
			)
			records = append(records, setIDRecord)

			hash32 := [32]byte(htlc.AMP.Hash)
			hashRecord := tlv.MakePrimitiveRecord(
				htlcHashType, &hash32,
			)
			records = append(records, hashRecord)

			if htlc.AMP.Preimage != nil {
				preimage32 := [32]byte(*htlc.AMP.Preimage)
				preimageRecord := tlv.MakePrimitiveRecord(
					htlcPreimageType, &preimage32,
				)
				records = append(records, preimageRecord)
			}
		}
		records = append(records, tlv.MakePrimitiveRecord(assetIdType, &assetId))

		// Convert the custom records to tlv.Record types that are ready
		// for serialization.
		customRecords := tlv.MapToRecords(htlc.CustomRecords)

		// Append the custom records. Their ids are in the experimental
		// range and sorted, so there is no need to sort again.
		records = append(records, customRecords...)

		tlvStream, err := tlv.NewStream(records...)
		if err != nil {
			return err
		}

		var b bytes.Buffer
		if err := tlvStream.Encode(&b); err != nil {
			return err
		}

		// Write the length of the tlv stream followed by the stream
		// bytes.
		err = binary.Write(w, byteOrder, uint64(b.Len()))
		if err != nil {
			return err
		}

		if _, err := w.Write(b.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

// putNanoTime returns the unix nano time for the passed timestamp. A zero-value
// timestamp will be mapped to 0, since calling UnixNano in that case is
// undefined.
func putNanoTime(t time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	return uint64(t.UnixNano())
}

// getNanoTime returns a timestamp for the given number of nano seconds. If zero
// is provided, an zero-value time stamp is returned.
func getNanoTime(ns uint64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(ns))
}

// fetchInvoiceStateAMP retrieves the state of all the relevant sub-invoice for
// an AMP invoice. This methods only decode the relevant state vs the entire
// invoice.
func fetchInvoiceStateAMP(invoiceNum []byte,
	invoices kvdb.RBucket) (AMPInvoiceState, error) {

	// Fetch the raw invoice bytes.
	invoiceBytes := invoices.Get(invoiceNum)
	if invoiceBytes == nil {
		return nil, ErrInvoiceNotFound
	}

	r := bytes.NewReader(invoiceBytes)

	var bodyLen int64
	err := binary.Read(r, byteOrder, &bodyLen)
	if err != nil {
		return nil, err
	}

	// Next, we'll make a new TLV stream that only attempts to decode the
	// bytes we actually need.
	ampState := make(AMPInvoiceState)
	tlvStream, err := tlv.NewStream(
		// Invoice AMP state.
		tlv.MakeDynamicRecord(
			invoiceAmpStateType, &ampState, nil,
			ampStateEncoder, ampStateDecoder,
		),
	)
	if err != nil {
		return nil, err
	}

	invoiceReader := io.LimitReader(r, bodyLen)
	if err = tlvStream.Decode(invoiceReader); err != nil {
		return nil, err
	}

	return ampState, nil
}

func encodeCircuitKeys(w io.Writer, val interface{}, buf *[8]byte) error {
	if v, ok := val.(*map[CircuitKey]struct{}); ok {
		// We encode the set of circuit keys as a varint length prefix.
		// followed by a series of fixed sized uint8 integers.
		numKeys := uint64(len(*v))

		if err := tlv.WriteVarInt(w, numKeys, buf); err != nil {
			return err
		}

		for key := range *v {
			scidInt := key.ChanID.ToUint64()

			if err := tlv.EUint64(w, &scidInt, buf); err != nil {
				return err
			}
			if err := tlv.EUint64(w, &key.HtlcID, buf); err != nil {
				return err
			}
		}

		return nil
	}

	return tlv.NewTypeForEncodingErr(val, "*map[CircuitKey]struct{}")
}

func decodeCircuitKeys(r io.Reader, val interface{}, buf *[8]byte, l uint64) error {
	if v, ok := val.(*map[CircuitKey]struct{}); ok {
		// First, we'll read out the varint that encodes the number of
		// circuit keys encoded.
		numKeys, err := tlv.ReadVarInt(r, buf)
		if err != nil {
			return err
		}

		// Now that we know how many keys to expect, iterate reading each
		// one until we're done.
		for i := uint64(0); i < numKeys; i++ {
			var (
				key  CircuitKey
				scid uint64
			)

			if err := tlv.DUint64(r, &scid, buf, 8); err != nil {
				return err
			}

			key.ChanID = lnwire.NewShortChanIDFromInt(scid)

			if err := tlv.DUint64(r, &key.HtlcID, buf, 8); err != nil {
				return err
			}

			(*v)[key] = struct{}{}
		}

		return nil
	}

	return tlv.NewTypeForDecodingErr(val, "*map[CircuitKey]struct{}", l, l)
}

// ampStateEncoder is a custom TLV encoder for the AMPInvoiceState record.
func ampStateEncoder(w io.Writer, val interface{}, buf *[8]byte) error {
	if v, ok := val.(*AMPInvoiceState); ok {
		// We'll encode the AMP state as a series of KV pairs on the
		// wire with a length prefix.
		numRecords := uint64(len(*v))

		// First, we'll write out the number of records as a var int.
		if err := tlv.WriteVarInt(w, numRecords, buf); err != nil {
			return err
		}

		// With that written out, we'll now encode the entries
		// themselves as a sub-TLV record, which includes its _own_
		// inner length prefix.
		for setID, ampState := range *v {
			setID := [32]byte(setID)
			ampState := ampState

			htlcState := uint8(ampState.State)
			settleDateBytes, err := ampState.SettleDate.MarshalBinary()
			if err != nil {
				return err
			}

			amtPaid := uint64(ampState.AmtPaid)

			var ampStateTlvBytes bytes.Buffer
			tlvStream, err := tlv.NewStream(
				tlv.MakePrimitiveRecord(
					ampStateSetIDType, &setID,
				),
				tlv.MakePrimitiveRecord(
					ampStateHtlcStateType, &htlcState,
				),
				tlv.MakePrimitiveRecord(
					ampStateSettleIndexType, &ampState.SettleIndex,
				),
				tlv.MakePrimitiveRecord(
					ampStateSettleDateType, &settleDateBytes,
				),
				tlv.MakeDynamicRecord(
					ampStateCircuitKeysType,
					&ampState.InvoiceKeys,
					func() uint64 {
						// The record takes 8 bytes to encode the
						// set of circuits,  8 bytes for the scid
						// for the key, and 8 bytes for the HTLC
						// index.
						numKeys := uint64(len(ampState.InvoiceKeys))
						return tlv.VarIntSize(numKeys) + (numKeys * 16)
					},
					encodeCircuitKeys, decodeCircuitKeys,
				),
				tlv.MakePrimitiveRecord(
					ampStateAmtPaidType, &amtPaid,
				),
				tlv.MakePrimitiveRecord(
					ampStateAssetIdType, &ampState.AssetId,
				),
			)
			if err != nil {
				return err
			}

			if err := tlvStream.Encode(&ampStateTlvBytes); err != nil {
				return err
			}

			// We encode the record with a varint length followed by
			// the _raw_ TLV bytes.
			tlvLen := uint64(len(ampStateTlvBytes.Bytes()))
			if err := tlv.WriteVarInt(w, tlvLen, buf); err != nil {
				return err
			}

			if _, err := w.Write(ampStateTlvBytes.Bytes()); err != nil {
				return err
			}
		}

		return nil
	}

	return tlv.NewTypeForEncodingErr(val, "channeldb.AMPInvoiceState")
}

// ampStateDecoder is a custom TLV decoder for the AMPInvoiceState record.
func ampStateDecoder(r io.Reader, val interface{}, buf *[8]byte, l uint64) error {
	if v, ok := val.(*AMPInvoiceState); ok {
		// First, we'll decode the varint that encodes how many set IDs
		// are encoded within the greater map.
		numRecords, err := tlv.ReadVarInt(r, buf)
		if err != nil {
			return err
		}

		// Now that we know how many records we'll need to read, we can
		// iterate and read them all out in series.
		for i := uint64(0); i < numRecords; i++ {
			// Read out the varint that encodes the size of this inner
			// TLV record
			stateRecordSize, err := tlv.ReadVarInt(r, buf)
			if err != nil {
				return err
			}

			// Using this information, we'll create a new limited
			// reader that'll return an EOF once the end has been
			// reached so the stream stops consuming bytes.
			innerTlvReader := io.LimitedReader{
				R: r,
				N: int64(stateRecordSize),
			}

			var (
				setID           [32]byte
				htlcState       uint8
				settleIndex     uint64
				settleDateBytes []byte
				invoiceKeys     = make(map[CircuitKey]struct{})
				amtPaid         uint64
				assetId         uint32
			)
			tlvStream, err := tlv.NewStream(
				tlv.MakePrimitiveRecord(
					ampStateSetIDType, &setID,
				),
				tlv.MakePrimitiveRecord(
					ampStateHtlcStateType, &htlcState,
				),
				tlv.MakePrimitiveRecord(
					ampStateSettleIndexType, &settleIndex,
				),
				tlv.MakePrimitiveRecord(
					ampStateSettleDateType, &settleDateBytes,
				),
				tlv.MakeDynamicRecord(
					ampStateCircuitKeysType,
					&invoiceKeys, nil,
					encodeCircuitKeys, decodeCircuitKeys,
				),
				tlv.MakePrimitiveRecord(
					ampStateAmtPaidType, &amtPaid,
				),
				tlv.MakePrimitiveRecord(
					ampStateAssetIdType, &assetId,
				),
			)
			if err != nil {
				return err
			}

			if err := tlvStream.Decode(&innerTlvReader); err != nil {
				return err
			}

			var settleDate time.Time
			err = settleDate.UnmarshalBinary(settleDateBytes)
			if err != nil {
				return err
			}

			(*v)[setID] = InvoiceStateAMP{
				State:       HtlcState(htlcState),
				SettleIndex: settleIndex,
				SettleDate:  settleDate,
				InvoiceKeys: invoiceKeys,
				AmtPaid:     lnwire.UnitPrec11(amtPaid),
				AssetId:     assetId,
			}
		}

		return nil
	}

	return tlv.NewTypeForEncodingErr(val, "channeldb.AMPInvoiceState")
}

//
//// deserializeHtlcs reads a list of invoice htlcs from a reader and returns it
//// as a map.
//func deserializeHtlcs(r io.Reader) (map[CircuitKey]*InvoiceHTLC, error) {
//	htlcs := make(map[CircuitKey]*InvoiceHTLC)
//
//	for {
//		// Read the length of the tlv stream for this htlc.
//		var streamLen int64
//		if err := binary.Read(r, byteOrder, &streamLen); err != nil {
//			if err == io.EOF {
//				break
//			}
//
//			return nil, err
//		}
//
//		// Limit the reader so that it stops at the end of this htlc's
//		// stream.
//		htlcReader := io.LimitReader(r, streamLen)
//
//		// Decode the contents into the htlc fields.
//		var (
//			htlc                    InvoiceHTLC
//			key                     CircuitKey
//			chanID                  uint64
//			state                   uint8
//			acceptTime, resolveTime uint64
//			amt, mppTotalAmt        uint64
//			assetId                 uint64
//			amp                     = &record.AMP{}
//			hash32                  = &[32]byte{}
//			preimage32              = &[32]byte{}
//		)
//		tlvStream, err := tlv.NewStream(
//			tlv.MakePrimitiveRecord(chanIDType, &chanID),
//			tlv.MakePrimitiveRecord(htlcIDType, &key.HtlcID),
//			tlv.MakePrimitiveRecord(amtType, &amt),
//			tlv.MakePrimitiveRecord(
//				acceptHeightType, &htlc.AcceptHeight,
//			),
//			tlv.MakePrimitiveRecord(acceptTimeType, &acceptTime),
//			tlv.MakePrimitiveRecord(resolveTimeType, &resolveTime),
//			tlv.MakePrimitiveRecord(expiryHeightType, &htlc.Expiry),
//			tlv.MakePrimitiveRecord(htlcStateType, &state),
//			tlv.MakePrimitiveRecord(mppTotalAmtType, &mppTotalAmt),
//			tlv.MakeDynamicRecord(
//				htlcAMPType, amp, amp.PayloadSize,
//				record.AMPEncoder, record.AMPDecoder,
//			),
//			tlv.MakePrimitiveRecord(htlcHashType, hash32),
//			tlv.MakePrimitiveRecord(htlcPreimageType, preimage32),
//			tlv.MakePrimitiveRecord(assetIdType, &assetId),
//		)
//		if err != nil {
//			return nil, err
//		}
//
//		parsedTypes, err := tlvStream.DecodeWithParsedTypes(htlcReader)
//		if err != nil {
//			return nil, err
//		}
//
//		if _, ok := parsedTypes[htlcAMPType]; !ok {
//			amp = nil
//		}
//
//		var preimage *lntypes.Preimage
//		if _, ok := parsedTypes[htlcPreimageType]; ok {
//			pimg := lntypes.Preimage(*preimage32)
//			preimage = &pimg
//		}
//
//		var hash *lntypes.Hash
//		if _, ok := parsedTypes[htlcHashType]; ok {
//			h := lntypes.Hash(*hash32)
//			hash = &h
//		}
//
//		key.ChanID = lnwire.NewShortChanIDFromInt(chanID)
//		htlc.AcceptTime = getNanoTime(acceptTime)
//		htlc.ResolveTime = getNanoTime(resolveTime)
//		htlc.State = HtlcState(state)
//		htlc.Amt = lnwire.UnitPrec11(amt)
//		htlc.AssetId = uint32(assetId)
//		htlc.MppTotalAmt = lnwire.UnitPrec11(mppTotalAmt)
//		if amp != nil && hash != nil {
//			htlc.AMP = &InvoiceHtlcAMPData{
//				Record:   *amp,
//				Hash:     *hash,
//				Preimage: preimage,
//			}
//		}
//
//		// Reconstruct the custom records fields from the parsed types
//		// map return from the tlv parser.
//		htlc.CustomRecords = hop.NewCustomRecords(parsedTypes)
//
//		htlcs[key] = &htlc
//	}
//
//	return htlcs, nil
//}

// copySlice allocates a new slice and copies the source into it.
func copySlice(src []byte) []byte {
	dest := make([]byte, len(src))
	copy(dest, src)
	return dest
}

// copyInvoice makes a deep copy of the supplied invoice.
func copyInvoice(src *Invoice) *Invoice {
	dest := Invoice{
		Memo:           copySlice(src.Memo),
		PaymentRequest: copySlice(src.PaymentRequest),
		CreationDate:   src.CreationDate,
		SettleDate:     src.SettleDate,
		Terms:          src.Terms,
		AddIndex:       src.AddIndex,
		SettleIndex:    src.SettleIndex,
		State:          src.State,
		AmtPaid:        src.AmtPaid,
		Htlcs: make(
			map[CircuitKey]*InvoiceHTLC, len(src.Htlcs),
		),
		HodlInvoice: src.HodlInvoice,
	}

	dest.Terms.Features = src.Terms.Features.Clone()

	if src.Terms.PaymentPreimage != nil {
		preimage := *src.Terms.PaymentPreimage
		dest.Terms.PaymentPreimage = &preimage
	}

	for k, v := range src.Htlcs {
		dest.Htlcs[k] = v.Copy()
	}

	return &dest
}

// invoiceSetIDKeyLen is the length of the key that's used to store the
// individual HTLCs prefixed by their ID along side the main invoice within the
// invoiceBytes. We use 4 bytes for the invoice number, and 32 bytes for the
// set ID.
const invoiceSetIDKeyLen = 4 + 32

// InvoiceDeleteRef holds a reference to an invoice to be deleted.
type InvoiceDeleteRef struct {
	// PayHash is the payment hash of the target invoice. All invoices are
	// currently indexed by payment hash.
	PayHash lntypes.Hash

	// PayAddr is the payment addr of the target invoice. Newer invoices
	// (0.11 and up) are indexed by payment address in addition to payment
	// hash, but pre 0.8 invoices do not have one at all.
	PayAddr *[32]byte

	// AddIndex is the add index of the invoice.
	AddIndex uint64

	// SettleIndex is the settle index of the invoice.
	SettleIndex uint64
}
