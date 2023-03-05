package channeldb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/tlv"
	"io"
	"sort"
	"time"

	"github.com/SeFo-Finance/obd-go-bindings/lnwire"
	"github.com/SeFo-Finance/obd-go-bindings/record"
	"github.com/SeFo-Finance/obd-go-bindings/routing/route"

	"github.com/lightningnetwork/lnd/lntypes"
)

var (
	// paymentsRootBucket is the name of the top-level bucket within the
	// database that stores all data related to payments. Within this
	// bucket, each payment hash its own sub-bucket keyed by its payment
	// hash.
	//
	// Bucket hierarchy:
	//
	// root-bucket
	//      |
	//      |-- <paymenthash>
	//      |        |--sequence-key: <sequence number>
	//      |        |--creation-info-key: <creation info>
	//      |        |--fail-info-key: <(optional) fail info>
	//      |        |
	//      |        |--payment-htlcs-bucket (shard-bucket)
	//      |        |        |
	//      |        |        |-- ai<htlc attempt ID>: <htlc attempt info>
	//      |        |        |-- si<htlc attempt ID>: <(optional) settle info>
	//      |        |        |-- fi<htlc attempt ID>: <(optional) fail info>
	//      |        |        |
	//      |        |       ...
	//      |        |
	//      |        |
	//      |        |--duplicate-bucket (only for old, completed payments)
	//      |                 |
	//      |                 |-- <seq-num>
	//      |                 |       |--sequence-key: <sequence number>
	//      |                 |       |--creation-info-key: <creation info>
	//      |                 |       |--ai: <attempt info>
	//      |                 |       |--si: <settle info>
	//      |                 |       |--fi: <fail info>
	//      |                 |
	//      |                 |-- <seq-num>
	//      |                 |       |
	//      |                ...     ...
	//      |
	//      |-- <paymenthash>
	//      |        |
	//      |       ...
	//     ...
	//
	paymentsRootBucket = []byte("payments-root-bucket")

	// paymentSequenceKey is a key used in the payment's sub-bucket to
	// store the sequence number of the payment.
	paymentSequenceKey = []byte("payment-sequence-key")

	// paymentCreationInfoKey is a key used in the payment's sub-bucket to
	// store the creation info of the payment.
	paymentCreationInfoKey = []byte("payment-creation-info")

	// paymentHtlcsBucket is a bucket where we'll store the information
	// about the HTLCs that were attempted for a payment.
	paymentHtlcsBucket = []byte("payment-htlcs-bucket")

	// htlcAttemptInfoKey is the key used as the prefix of an HTLC attempt
	// to store the info about the attempt that was done for the HTLC in
	// question. The HTLC attempt ID is concatenated at the end.
	htlcAttemptInfoKey = []byte("ai")

	// htlcSettleInfoKey is the key used as the prefix of an HTLC attempt
	// settle info, if any. The HTLC attempt ID is concatenated at the end.
	htlcSettleInfoKey = []byte("si")

	// htlcFailInfoKey is the key used as the prefix of an HTLC attempt
	// failure information, if any.The  HTLC attempt ID is concatenated at
	// the end.
	htlcFailInfoKey = []byte("fi")

	// paymentFailInfoKey is a key used in the payment's sub-bucket to
	// store information about the reason a payment failed.
	paymentFailInfoKey = []byte("payment-fail-info")

	// paymentsIndexBucket is the name of the top-level bucket within the
	// database that stores an index of payment sequence numbers to its
	// payment hash.
	// payments-sequence-index-bucket
	// 	|--<sequence-number>: <payment hash>
	// 	|--...
	// 	|--<sequence-number>: <payment hash>
	paymentsIndexBucket = []byte("payments-index-bucket")
)

var (
	// ErrNoSequenceNumber is returned if we lookup a payment which does
	// not have a sequence number.
	ErrNoSequenceNumber = errors.New("sequence number not found")

	// ErrDuplicateNotFound is returned when we lookup a payment by its
	// index and cannot find a payment with a matching sequence number.
	ErrDuplicateNotFound = errors.New("duplicate payment not found")

	// ErrNoDuplicateBucket is returned when we expect to find duplicates
	// when looking up a payment from its index, but the payment does not
	// have any.
	ErrNoDuplicateBucket = errors.New("expected duplicate bucket")

	// ErrNoDuplicateNestedBucket is returned if we do not find duplicate
	// payments in their own sub-bucket.
	ErrNoDuplicateNestedBucket = errors.New("nested duplicate bucket not " +
		"found")
)

// FailureReason encodes the reason a payment ultimately failed.
type FailureReason byte

const (
	// FailureReasonTimeout indicates that the payment did timeout before a
	// successful payment attempt was made.
	FailureReasonTimeout FailureReason = 0

	// FailureReasonNoRoute indicates no successful route to the
	// destination was found during path finding.
	FailureReasonNoRoute FailureReason = 1

	// FailureReasonError indicates that an unexpected error happened during
	// payment.
	FailureReasonError FailureReason = 2

	// FailureReasonPaymentDetails indicates that either the hash is unknown
	// or the final cltv delta or amount is incorrect.
	FailureReasonPaymentDetails FailureReason = 3

	// FailureReasonInsufficientBalance indicates that we didn't have enough
	// balance to complete the payment.
	FailureReasonInsufficientBalance FailureReason = 4

	// TODO(halseth): cancel state.

	// TODO(joostjager): Add failure reasons for:
	// LocalLiquidityInsufficient, RemoteCapacityInsufficient.
)

// Error returns a human readable error string for the FailureReason.
func (r FailureReason) Error() string {
	return r.String()
}

// String returns a human readable FailureReason.
func (r FailureReason) String() string {
	switch r {
	case FailureReasonTimeout:
		return "timeout"
	case FailureReasonNoRoute:
		return "no_route"
	case FailureReasonError:
		return "error"
	case FailureReasonPaymentDetails:
		return "incorrect_payment_details"
	case FailureReasonInsufficientBalance:
		return "insufficient_balance"
	}

	return "unknown"
}

// PaymentStatus represent current status of payment
type PaymentStatus byte

const (
	// StatusUnknown is the status where a payment has never been initiated
	// and hence is unknown.
	StatusUnknown PaymentStatus = 0

	// StatusInFlight is the status where a payment has been initiated, but
	// a response has not been received.
	StatusInFlight PaymentStatus = 1

	// StatusSucceeded is the status where a payment has been initiated and
	// the payment was completed successfully.
	StatusSucceeded PaymentStatus = 2

	// StatusFailed is the status where a payment has been initiated and a
	// failure result has come back.
	StatusFailed PaymentStatus = 3
)

// String returns readable representation of payment status.
func (ps PaymentStatus) String() string {
	switch ps {
	case StatusUnknown:
		return "Unknown"
	case StatusInFlight:
		return "In Flight"
	case StatusSucceeded:
		return "Succeeded"
	case StatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// PaymentCreationInfo is the information necessary to have ready when
// initiating a payment, moving it into state InFlight.
type PaymentCreationInfo struct {
	// PaymentIdentifier is the hash this payment is paying to in case of
	// non-AMP payments, and the SetID for AMP payments.
	PaymentIdentifier lntypes.Hash

	// Value is the amount we are paying.
	//Value lnwire.MilliSatoshi
	Value   lnwire.UnitPrec11
	AssetId uint32

	// CreationTime is the time when this payment was initiated.
	CreationTime time.Time

	// PaymentRequest is the full payment request, if any.
	PaymentRequest []byte
}

//
//// htlcBucketKey creates a composite key from prefix and id where the result is
//// simply the two concatenated.
//func htlcBucketKey(prefix, id []byte) []byte {
//	key := make([]byte, len(prefix)+len(id))
//	copy(key, prefix)
//	copy(key[len(prefix):], id)
//	return key
//}

// fetchHtlcAttempts retrives all htlc attempts made for the payment found in
// the given bucket.
func fetchHtlcAttempts(bucket kvdb.RBucket) ([]HTLCAttempt, error) {
	htlcsMap := make(map[uint64]*HTLCAttempt)

	attemptInfoCount := 0
	err := bucket.ForEach(func(k, v []byte) error {
		aid := byteOrder.Uint64(k[len(k)-8:])

		if _, ok := htlcsMap[aid]; !ok {
			htlcsMap[aid] = &HTLCAttempt{}
		}

		var err error
		switch {
		case bytes.HasPrefix(k, htlcAttemptInfoKey):
			attemptInfo, err := readHtlcAttemptInfo(v)
			if err != nil {
				return err
			}

			attemptInfo.AttemptID = aid
			htlcsMap[aid].HTLCAttemptInfo = *attemptInfo
			attemptInfoCount++

		case bytes.HasPrefix(k, htlcSettleInfoKey):
			htlcsMap[aid].Settle, err = readHtlcSettleInfo(v)
			if err != nil {
				return err
			}

		case bytes.HasPrefix(k, htlcFailInfoKey):
			htlcsMap[aid].Failure, err = readHtlcFailInfo(v)
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown htlc attempt key")
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sanity check that all htlcs have an attempt info.
	if attemptInfoCount != len(htlcsMap) {
		return nil, errNoAttemptInfo
	}

	keys := make([]uint64, len(htlcsMap))
	i := 0
	for k := range htlcsMap {
		keys[i] = k
		i++
	}

	// Sort HTLC attempts by their attempt ID. This is needed because in the
	// DB we store the attempts with keys prefixed by their status which
	// changes order (groups them together by status).
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	htlcs := make([]HTLCAttempt, len(htlcsMap))
	for i, key := range keys {
		htlcs[i] = *htlcsMap[key]
	}

	return htlcs, nil
}

// readHtlcAttemptInfo reads the payment attempt info for this htlc.
func readHtlcAttemptInfo(b []byte) (*HTLCAttemptInfo, error) {
	r := bytes.NewReader(b)
	return deserializeHTLCAttemptInfo(r)
}

// readHtlcSettleInfo reads the settle info for the htlc. If the htlc isn't
// settled, nil is returned.
func readHtlcSettleInfo(b []byte) (*HTLCSettleInfo, error) {
	r := bytes.NewReader(b)
	return deserializeHTLCSettleInfo(r)
}

// readHtlcFailInfo reads the failure info for the htlc. If the htlc hasn't
// failed, nil is returned.
func readHtlcFailInfo(b []byte) (*HTLCFailInfo, error) {
	r := bytes.NewReader(b)
	return deserializeHTLCFailInfo(r)
}

// fetchFailedHtlcKeys retrieves the bucket keys of all failed HTLCs of a
// payment bucket.
func fetchFailedHtlcKeys(bucket kvdb.RBucket) ([][]byte, error) {
	htlcsBucket := bucket.NestedReadBucket(paymentHtlcsBucket)

	var htlcs []HTLCAttempt
	var err error
	if htlcsBucket != nil {
		htlcs, err = fetchHtlcAttempts(htlcsBucket)
		if err != nil {
			return nil, err
		}
	}

	// Now iterate though them and save the bucket keys for the failed
	// HTLCs.
	var htlcKeys [][]byte
	for _, h := range htlcs {
		if h.Failure == nil {
			continue
		}

		htlcKeyBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(htlcKeyBytes, h.AttemptID)

		htlcKeys = append(htlcKeys, htlcKeyBytes)
	}

	return htlcKeys, nil
}

// PaymentsQuery represents a query to the payments database starting or ending
// at a certain offset index. The number of retrieved records can be limited.
type PaymentsQuery struct {
	// IndexOffset determines the starting point of the payments query and
	// is always exclusive. In normal order, the query starts at the next
	// higher (available) index compared to IndexOffset. In reversed order,
	// the query ends at the next lower (available) index compared to the
	// IndexOffset. In the case of a zero index_offset, the query will start
	// with the oldest payment when paginating forwards, or will end with
	// the most recent payment when paginating backwards.
	IndexOffset uint64

	// MaxPayments is the maximal number of payments returned in the
	// payments query.
	MaxPayments uint64

	// Reversed gives a meaning to the IndexOffset. If reversed is set to
	// true, the query will fetch payments with indices lower than the
	// IndexOffset, otherwise, it will return payments with indices greater
	// than the IndexOffset.
	Reversed bool

	// If IncludeIncomplete is true, then return payments that have not yet
	// fully completed. This means that pending payments, as well as failed
	// payments will show up if this field is set to true.
	IncludeIncomplete bool

	/* obd update wxf
	 *query payments for special asset,set it to true.
	 */
	IsQueryAsset bool
	AssetId      uint32
	StartTime    int64
	EndTime      int64
}

// PaymentsResponse contains the result of a query to the payments database.
// It includes the set of payments that match the query and integers which
// represent the index of the first and last item returned in the series of
// payments. These integers allow callers to resume their query in the event
// that the query's response exceeds the max number of returnable events.
type PaymentsResponse struct {
	// Payments is the set of payments returned from the database for the
	// PaymentsQuery.
	Payments []*MPPayment

	// FirstIndexOffset is the index of the first element in the set of
	// returned MPPayments. Callers can use this to resume their query
	// in the event that the slice has too many events to fit into a single
	// response. The offset can be used to continue reverse pagination.
	FirstIndexOffset uint64

	// LastIndexOffset is the index of the last element in the set of
	// returned MPPayments. Callers can use this to resume their query
	// in the event that the slice has too many events to fit into a single
	// response. The offset can be used to continue forward pagination.
	LastIndexOffset uint64
}

// fetchSequenceNumbers fetches all the sequence numbers associated with a
// payment, including those belonging to any duplicate payments.
func fetchSequenceNumbers(paymentBucket kvdb.RBucket) ([][]byte, error) {
	seqNum := paymentBucket.Get(paymentSequenceKey)
	if seqNum == nil {
		return nil, errors.New("expected sequence number")
	}

	sequenceNumbers := [][]byte{seqNum}

	// Get the duplicate payments bucket, if it has no duplicates, just
	// return early with the payment sequence number.
	duplicates := paymentBucket.NestedReadBucket(duplicatePaymentsBucket)
	if duplicates == nil {
		return sequenceNumbers, nil
	}

	// If we do have duplicated, they are keyed by sequence number, so we
	// iterate through the duplicates bucket and add them to our set of
	// sequence numbers.
	if err := duplicates.ForEach(func(k, v []byte) error {
		sequenceNumbers = append(sequenceNumbers, k)
		return nil
	}); err != nil {
		return nil, err
	}

	return sequenceNumbers, nil
}

// nolint: dupl
func serializePaymentCreationInfo(w io.Writer, c *PaymentCreationInfo) error {
	var scratch [8]byte

	if _, err := w.Write(c.PaymentIdentifier[:]); err != nil {
		return err
	}

	byteOrder.PutUint64(scratch[:], uint64(c.Value))
	if _, err := w.Write(scratch[:]); err != nil {
		return err
	}

	/*obd update wxf
	add asset_id to serializePaymentCreationInfo*/
	byteOrder.PutUint32(scratch[:4], c.AssetId)
	if _, err := w.Write(scratch[:4]); err != nil {
		return err
	}

	if err := serializeTime(w, c.CreationTime); err != nil {
		return err
	}

	byteOrder.PutUint32(scratch[:4], uint32(len(c.PaymentRequest)))
	if _, err := w.Write(scratch[:4]); err != nil {
		return err
	}

	if _, err := w.Write(c.PaymentRequest[:]); err != nil {
		return err
	}

	return nil
}

func deserializePaymentCreationInfo(r io.Reader) (*PaymentCreationInfo, error) {
	var scratch [8]byte

	c := &PaymentCreationInfo{}

	if _, err := io.ReadFull(r, c.PaymentIdentifier[:]); err != nil {
		return nil, err
	}

	if _, err := io.ReadFull(r, scratch[:]); err != nil {
		return nil, err
	}
	c.Value = lnwire.UnitPrec11(byteOrder.Uint64(scratch[:]))

	/*obd update wxf
	add asset_id to deserializePaymentCreationInfo*/
	if _, err := io.ReadFull(r, scratch[:4]); err != nil {
		return nil, err
	}
	c.AssetId = byteOrder.Uint32(scratch[:4])

	creationTime, err := deserializeTime(r)
	if err != nil {
		return nil, err
	}
	c.CreationTime = creationTime

	if _, err := io.ReadFull(r, scratch[:4]); err != nil {
		return nil, err
	}

	reqLen := uint32(byteOrder.Uint32(scratch[:4]))
	payReq := make([]byte, reqLen)
	if reqLen > 0 {
		if _, err := io.ReadFull(r, payReq); err != nil {
			return nil, err
		}
	}
	c.PaymentRequest = payReq

	return c, nil
}

func serializeHTLCAttemptInfo(w io.Writer, a *HTLCAttemptInfo) error {
	if err := WriteElements(w, a.sessionKey); err != nil {
		return err
	}

	if err := SerializeRoute(w, a.Route); err != nil {
		return err
	}

	if err := serializeTime(w, a.AttemptTime); err != nil {
		return err
	}

	// If the hash is nil we can just return.
	if a.Hash == nil {
		return nil
	}

	if _, err := w.Write(a.Hash[:]); err != nil {
		return err
	}

	return nil
}

func deserializeHTLCAttemptInfo(r io.Reader) (*HTLCAttemptInfo, error) {
	a := &HTLCAttemptInfo{}
	err := ReadElements(r, &a.sessionKey)
	if err != nil {
		return nil, err
	}

	a.Route, err = DeserializeRoute(r)
	if err != nil {
		return nil, err
	}

	a.AttemptTime, err = deserializeTime(r)
	if err != nil {
		return nil, err
	}

	hash := lntypes.Hash{}
	_, err = io.ReadFull(r, hash[:])

	switch {

	// Older payment attempts wouldn't have the hash set, in which case we
	// can just return.
	case err == io.EOF, err == io.ErrUnexpectedEOF:
		return a, nil

	case err != nil:
		return nil, err

	default:
	}

	a.Hash = &hash

	return a, nil
}

func serializeHop(w io.Writer, h *route.Hop) error {
	if err := WriteElements(w,
		h.PubKeyBytes[:],
		h.ChannelID,
		h.OutgoingTimeLock,
		h.AmtToForward,
	); err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, h.LegacyPayload); err != nil {
		return err
	}

	// For legacy payloads, we don't need to write any TLV records, so
	// we'll write a zero indicating the our serialized TLV map has no
	// records.
	if h.LegacyPayload {
		return WriteElements(w, uint32(0))
	}

	// Gather all non-primitive TLV records so that they can be serialized
	// as a single blob.
	//
	// TODO(conner): add migration to unify all fields in a single TLV
	// blobs. The split approach will cause headaches down the road as more
	// fields are added, which we can avoid by having a single TLV stream
	// for all payload fields.
	var records []tlv.Record
	if h.MPP != nil {
		records = append(records, h.MPP.Record())
	}

	// Final sanity check to absolutely rule out custom records that are not
	// custom and write into the standard range.
	if err := h.CustomRecords.Validate(); err != nil {
		return err
	}

	// Convert custom records to tlv and add to the record list.
	// MapToRecords sorts the list, so adding it here will keep the list
	// canonical.
	tlvRecords := tlv.MapToRecords(h.CustomRecords)
	records = append(records, tlvRecords...)

	// Otherwise, we'll transform our slice of records into a map of the
	// raw bytes, then serialize them in-line with a length (number of
	// elements) prefix.
	mapRecords, err := tlv.RecordsToMap(records)
	if err != nil {
		return err
	}

	numRecords := uint32(len(mapRecords))
	if err := WriteElements(w, numRecords); err != nil {
		return err
	}

	for recordType, rawBytes := range mapRecords {
		if err := WriteElements(w, recordType); err != nil {
			return err
		}

		if err := wire.WriteVarBytes(w, 0, rawBytes); err != nil {
			return err
		}
	}

	return nil
}

// maxOnionPayloadSize is the largest Sphinx payload possible, so we don't need
// to read/write a TLV stream larger than this.
const maxOnionPayloadSize = 1300

func deserializeHop(r io.Reader) (*route.Hop, error) {
	h := &route.Hop{}

	var pub []byte
	if err := ReadElements(r, &pub); err != nil {
		return nil, err
	}
	copy(h.PubKeyBytes[:], pub)

	if err := ReadElements(r,
		&h.ChannelID, &h.OutgoingTimeLock, &h.AmtToForward,
	); err != nil {
		return nil, err
	}

	// TODO(roasbeef): change field to allow LegacyPayload false to be the
	// legacy default?
	err := binary.Read(r, byteOrder, &h.LegacyPayload)
	if err != nil {
		return nil, err
	}

	var numElements uint32
	if err := ReadElements(r, &numElements); err != nil {
		return nil, err
	}

	// If there're no elements, then we can return early.
	if numElements == 0 {
		return h, nil
	}

	tlvMap := make(map[uint64][]byte)
	for i := uint32(0); i < numElements; i++ {
		var tlvType uint64
		if err := ReadElements(r, &tlvType); err != nil {
			return nil, err
		}

		rawRecordBytes, err := wire.ReadVarBytes(
			r, 0, maxOnionPayloadSize, "tlv",
		)
		if err != nil {
			return nil, err
		}

		tlvMap[tlvType] = rawRecordBytes
	}

	// If the MPP type is present, remove it from the generic TLV map and
	// parse it back into a proper MPP struct.
	//
	// TODO(conner): add migration to unify all fields in a single TLV
	// blobs. The split approach will cause headaches down the road as more
	// fields are added, which we can avoid by having a single TLV stream
	// for all payload fields.
	mppType := uint64(record.MPPOnionType)
	if mppBytes, ok := tlvMap[mppType]; ok {
		delete(tlvMap, mppType)

		var (
			mpp    = &record.MPP{}
			mppRec = mpp.Record()
			r      = bytes.NewReader(mppBytes)
		)
		err := mppRec.Decode(r, uint64(len(mppBytes)))
		if err != nil {
			return nil, err
		}
		h.MPP = mpp
	}

	h.CustomRecords = tlvMap

	return h, nil
}

// SerializeRoute serializes a route.
func SerializeRoute(w io.Writer, r route.Route) error {
	if err := WriteElements(w, r.AssetId,
		r.TotalTimeLock, r.TotalAmount, r.SourcePubKey[:],
	); err != nil {
		return err
	}

	if err := WriteElements(w, uint32(len(r.Hops))); err != nil {
		return err
	}

	for _, h := range r.Hops {
		if err := serializeHop(w, h); err != nil {
			return err
		}
	}

	return nil
}

// DeserializeRoute deserializes a route.
func DeserializeRoute(r io.Reader) (route.Route, error) {
	rt := route.Route{}
	if err := ReadElements(r, &rt.AssetId,
		&rt.TotalTimeLock, &rt.TotalAmount,
	); err != nil {
		return rt, err
	}

	var pub []byte
	if err := ReadElements(r, &pub); err != nil {
		return rt, err
	}
	copy(rt.SourcePubKey[:], pub)

	var numHops uint32
	if err := ReadElements(r, &numHops); err != nil {
		return rt, err
	}

	var hops []*route.Hop
	for i := uint32(0); i < numHops; i++ {
		hop, err := deserializeHop(r)
		if err != nil {
			return rt, err
		}
		hops = append(hops, hop)
	}
	rt.Hops = hops

	return rt, nil
}
