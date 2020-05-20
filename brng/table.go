package brng

// The goal of BRNG is to generate a batch of biased random numbers. At the end
// of running the BRNG protocol successfully, we should have `b` biased random
// numbers (also called the `batch size` of the BRNGer).
//
// Each of those biased random numbers is produced by the contribution of shares
// from all players participating in the protocol. Generally, we would say, `n`
// players contribute a set of `n` shares for a random number, such that each
// random number is represented by `k-1` degree polynomial.
//
// The protocol can be visualised by the illustration below.
//
//                            Slice
//                              |
//                           ___|__________________
//                         /    |   /|/|           /|
//                       /      V / /| | <-- Col /  |
//                     /        / /  | |       /    |
//                   /_______ /_/____|_|____ /     /|
//                   |       | |     | |    |    / /|
//                ^  |       | |     | |    |  / / <--- Row
//                |  |_______|_|_____|_|____|/ /    |
//           From |  |_|_E_|_|_|_|_|_|_|_|__|/      |
//                |  |       | |     | |    |       |
//                   |       | |    / /     |      /
//                   |       | |  / /       |    /   Batch
//                   |       | |/ /         |  /
//                   |_______|/|/___________|/
//                          ------>
//                            To
//
// Sharing holds the set of verifiable shares from a single player representing
// a single random number.
//
// Row defines a batch of Sharings, all coming from a single player. So a row
// would hold the `b` sets of verifiable shares, basically, the player's potential
// contribution for `b` biased random numbers.
//
// Element is a single verifiable share, marked as `E` in the above diagram. We
// therefore require a `from` field in an element, to tell us which player this
// verifiable share comes from.
//
// Col defines a list of elements, but specific to a particular index. It holds
// the jth share from each of the players.
//
// Slice is vertical slice of the above cube. It represents shares from all players
// for a specific index (Col) and `b` such Cols. Therefore a slice is basically
// a list of Cols.

import (
	"errors"
	"fmt"
	"io"

	"github.com/renproject/secp256k1-go"
	"github.com/renproject/shamir"
	"github.com/renproject/surge"
)

// A Sharing represents the shares and commitment generated by one party for
// one random number.
type Sharing struct {
	shares     shamir.VerifiableShares
	commitment shamir.Commitment
}

// NewSharing constructs a new Sharing from the given shares and commitment.
func NewSharing(shares shamir.VerifiableShares, commitment shamir.Commitment) Sharing {
	return Sharing{shares, commitment}
}

// SizeHint implements the surge.SizeHinter interface.
func (sharing Sharing) SizeHint() int {
	return sharing.shares.SizeHint() + sharing.commitment.SizeHint()
}

// Marshal implements the surge.Marshaler interface.
func (sharing Sharing) Marshal(w io.Writer, m int) (int, error) {
	m, err := sharing.shares.Marshal(w, m)
	if err != nil {
		return m, fmt.Errorf("marshaling shares: %v", err)
	}
	m, err = sharing.commitment.Marshal(w, m)
	if err != nil {
		return m, fmt.Errorf("marshaling commitment: %v", err)
	}
	return m, nil
}

// Unmarshal implements the surge.Unmarshaler interface.
func (sharing *Sharing) Unmarshal(r io.Reader, m int) (int, error) {
	m, err := sharing.shares.Unmarshal(r, m)
	if err != nil {
		return m, fmt.Errorf("unmarshaling shares: %v", err)
	}
	m, err = sharing.commitment.Unmarshal(r, m)
	if err != nil {
		return m, fmt.Errorf("unmarshaling commitment: %v", err)
	}
	return m, nil
}

// Shares returns the underlying shares of the sharing.
//
// NOTE: Modifying this return value will also modify the sharing.
func (sharing Sharing) Shares() shamir.VerifiableShares {
	return sharing.shares
}

// Commitment returns the underlying commitment of the sharing.
//
// NOTE: Modifying this return value will also modify the sharing.
func (sharing Sharing) Commitment() shamir.Commitment {
	return sharing.commitment
}

// ShareWithIndex returns the share in the Sharing with the given index, or an
// error if there is no share with the given index.
func (sharing Sharing) ShareWithIndex(index secp256k1.Secp256k1N) (shamir.VerifiableShare, error) {
	for _, share := range sharing.shares {
		s := share.Share()
		if s.IndexEq(&index) {
			return share, nil
		}
	}
	return shamir.VerifiableShare{}, errors.New("no share with the given index was found")
}

// N returns the number of shares in the Sharing.
func (sharing Sharing) N() int { return len(sharing.shares) }

// A Row represents a batch of Sharings that one player generates during BRNG.
type Row []Sharing

// SizeHint implements the surge.SizeHinter interface.
func (row Row) SizeHint() int { return surge.SizeHint(row) }

// Marshal implements the surge.Marshaler interface.
func (row Row) Marshal(w io.Writer, m int) (int, error) {
	return surge.Marshal(w, row, m)
}

// Unmarshal implements the surge.Unmarshaler interface.
func (row *Row) Unmarshal(r io.Reader, m int) (int, error) {
	return surge.Unmarshal(r, row, m)
}

// MakeRow allocates and returns a new empty row.
func MakeRow(n, k, b int) Row {
	sharings := make([]Sharing, b)
	for i := range sharings {
		sharings[i].shares = make(shamir.VerifiableShares, n)
		sharings[i].commitment = shamir.NewCommitmentWithCapacity(k)
	}

	return sharings
}

// BatchSize returns the batch number (the number of sharings) for the given
// Row.
func (row Row) BatchSize() int { return len(row) }

// N returns the number of shares in a any given Sharing of the given Row. If
// there are no sharings, or if not all of the sharings have the same number of
// shares, -1 is returned instead.
func (row Row) N() int {
	if row.BatchSize() == 0 {
		return -1
	}

	n := row[0].N()
	for i := 1; i < len(row); i++ {
		if row[i].N() != n {
			return -1
		}
	}

	return n
}

// An Element represents a share received from another player; along with the
// share, it contains the index of the player that created the sharing, and the
// assocaited Pedersen commitment.
type Element struct {
	from       secp256k1.Secp256k1N
	share      shamir.VerifiableShare
	commitment shamir.Commitment
}

// NewElement constructs a new Element from the given arguments.
func NewElement(
	from secp256k1.Secp256k1N,
	share shamir.VerifiableShare,
	commitment shamir.Commitment,
) Element {
	return Element{from, share, commitment}
}

// Share returns the share of the element
func (e Element) Share() shamir.VerifiableShare {
	return e.share
}

// Commitment returns the pedersen commitment for the
// share held in the element
func (e Element) Commitment() shamir.Commitment {
	return e.commitment
}

// SizeHint implements the surge.SizeHinter interface.
func (e Element) SizeHint() int {
	return e.from.SizeHint() + e.share.SizeHint() + e.commitment.SizeHint()
}

// Marshal implements the surge.Marshaler interface.
func (e Element) Marshal(w io.Writer, m int) (int, error) {
	m, err := e.from.Marshal(w, m)
	if err != nil {
		return m, fmt.Errorf("marshaling from: %v", err)
	}
	m, err = e.share.Marshal(w, m)
	if err != nil {
		return m, fmt.Errorf("marshaling share: %v", err)
	}
	m, err = e.commitment.Marshal(w, m)
	if err != nil {
		return m, fmt.Errorf("marshaling commitment: %v", err)
	}
	return m, nil
}

// Unmarshal implements the surge.Unmarshaler interface.
func (e Element) Unmarshal(r io.Reader, m int) (int, error) {
	m, err := e.from.Unmarshal(r, m)
	if err != nil {
		return m, fmt.Errorf("unmarshaling from: %v", err)
	}
	m, err = e.share.Unmarshal(r, m)
	if err != nil {
		return m, fmt.Errorf("unmarshaling share: %v", err)
	}
	m, err = e.commitment.Unmarshal(r, m)
	if err != nil {
		return m, fmt.Errorf("unmarshaling commitment: %v", err)
	}
	return m, nil
}

// Set the receiver to be equal to the given Element.
func (e *Element) Set(other Element) {
	e.from = other.from
	e.share = other.share
	e.commitment.Set(other.commitment)
}

// A Col is a slice of Elements, and represents all of the shares that
// correspond to a single global random number.
type Col []Element

// SizeHint implements the surge.SizeHinter interface.
func (col Col) SizeHint() int { return surge.SizeHint(col) }

// Marshal implements the surge.Marshaler interface.
func (col Col) Marshal(w io.Writer, m int) (int, error) {
	return surge.Marshal(w, col, m)
}

// Unmarshal implements the surge.Unmarshaler interface.
func (col *Col) Unmarshal(r io.Reader, m int) (int, error) {
	return surge.Unmarshal(r, col, m)
}

// HasValidForm return true if the given Col has the correct form, i.e. when it
// has at least one Element, and all shares for the Elements have the same
// index.
func (col Col) HasValidForm() bool {
	if len(col) == 0 {
		return false
	}

	share := col[0].share.Share()
	for i := 1; i < len(col); i++ {
		// FIXME: Create and use an IndexEq method on the
		// shamir.VerifiableShare type.
		s := col[i].share.Share()
		index := s.Index()
		if !share.IndexEq(&index) {
			return false
		}
	}

	return true
}

// Sum returns the share and Pedersen commitment that corresponds to the sum of
// the verifiable shares of the Elements in the Col.
func (col Col) Sum() (shamir.VerifiableShare, shamir.Commitment) {
	var share shamir.VerifiableShare
	var commitment shamir.Commitment

	if len(col) == 0 {
		return share, commitment
	}

	share = col[0].Share()
	commitment.Set(col[0].Commitment())

	for i := 1; i < len(col); i++ {
		share.Add(&share, &col[i].share)
		commitment.Add(&commitment, &col[i].commitment)
	}

	return share, commitment
}

// A Slice represents a batch of Cols, which corresponds to the batch number of
// global random numbers for the BRNG algorithm.
type Slice []Col

// SizeHint implements the surge.SizeHinter interface.
func (slice Slice) SizeHint() int { return surge.SizeHint(slice) }

// Marshal implements the surge.Marshaler interface.
func (slice Slice) Marshal(w io.Writer, m int) (int, error) {
	return surge.Marshal(w, slice, m)
}

// Unmarshal implements the surge.Unmarshaler interface.
func (slice *Slice) Unmarshal(r io.Reader, m int) (int, error) {
	return surge.Unmarshal(r, slice, m)
}

// BatchSize returns the number of Cols in the slice, which is equal to the
// batch size.
func (slice Slice) BatchSize() int {
	return len(slice)
}

// HasValidForm returns true
func (slice Slice) HasValidForm() bool {
	if slice.BatchSize() == 0 {
		return false
	}

	c0 := slice[0]
	if !c0.HasValidForm() {
		return false
	}
	// We can safely get the 0th element of c0 now beacuse HasValidForm
	// guarantees that there is at least one element.
	vshare := c0[0].Share()
	share := vshare.Share()
	index := share.Index()

	for i := 1; i < len(slice); i++ {
		if !slice[i].HasValidForm() {
			return false
		}

		// Check that the index is the same as for the first Col.
		vshare := slice[i][0].Share()
		share := vshare.Share()
		if !share.IndexEq(&index) {
			return false
		}
	}
	return true
}

// Faults returns a list of faults (if any) that exist in the given slice.
func (slice Slice) Faults(checker *shamir.VSSChecker) []Element {
	var faults []Element
	for _, c := range slice {
		for _, e := range c {
			if !checker.IsValid(&e.commitment, &e.share) {
				var fault Element
				fault.Set(e)
				faults = append(faults, fault)
			}
		}
	}

	if len(faults) == 0 {
		return nil
	}

	return faults
}

// A Table represents all of the shares across all players for a given run of
// the BRNG algorithm.
type Table []Row

// SizeHint implements the surge.SizeHinter interface.
func (t Table) SizeHint() int { return surge.SizeHint(t) }

// Marshal implements the surge.Marshaler interface.
func (t Table) Marshal(w io.Writer, m int) (int, error) {
	return surge.Marshal(w, t, m)
}

// Unmarshal implements the surge.Unmarshaler interface.
func (t *Table) Unmarshal(r io.Reader, m int) (int, error) {
	return surge.Unmarshal(r, t, m)
}

// Slice returns the Slice for the given index in the table.
func (t Table) Slice(index secp256k1.Secp256k1N, fromIndices []secp256k1.Secp256k1N) Slice {
	// NOTE: Assumes that the table is well formed.
	slice := make(Slice, t.BatchSize())
	for i := range slice {
		slice[i] = make(Col, t.Height())
	}

	// Get the integer index of the given index.
	ind := -1
	for i := range fromIndices {
		if index.Eq(&fromIndices[i]) {
			ind = i
		}
	}
	if ind == -1 {
		panic("index missing from fromIndices")
	}

	for i, row := range t {
		for j, sharing := range row {
			var commitment shamir.Commitment

			from := fromIndices[i]
			share := sharing.shares[ind]
			commitment.Set(sharing.Commitment())

			slice[j][i] = NewElement(from, share, commitment)
		}
	}

	return slice
}

// Height returns the number of different players that contributed rows to the
// table.
func (t Table) Height() int {
	return len(t)
}

// BatchSize returns the size of the batch of the table. If the table has no
// rows, or if not all of the rows have the same batch size, -1 is returned
// instead.
func (t Table) BatchSize() int {
	if t.Height() == 0 {
		return -1
	}

	b := len(t[0])
	for i := 1; i < len(t); i++ {
		if len(t[i]) != b {
			return -1
		}
	}

	return b
}

// HasValidDimensions returns true if each of the three dimensions of the table
// are valid and consistent. If any of the dimensions are 0, or if there are
// any inconsistencies in the dimensions, this function will return false.
func (t Table) HasValidDimensions() bool {
	if t.BatchSize() == -1 {
		return false
	}

	n := t[0].N()
	if n == -1 {
		return false
	}
	for i := 1; i < len(t); i++ {
		if t[i].N() != n {
			return false
		}
	}

	return true
}

// SizeHint implements the surge.SizeHinter interface
func (table Table) SizeHint() int {
	return surge.SizeHint(table)
}

// Marshal implements the surge.Marshaler interface.
func (table Table) Marshal(w io.Writer, m int) (int, error) {
	return surge.Marshal(w, table, m)
}

// Unmarshal implements the surge.Unmarshaler interface.
func (table *Table) Unmarshal(r io.Reader, m int) (int, error) {
	return surge.Unmarshal(r, table, m)
}
