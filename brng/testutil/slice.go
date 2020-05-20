package testutil

import (
	"math/rand"
	"sort"

	"github.com/renproject/mpc/brng"
	"github.com/renproject/secp256k1-go"
	"github.com/renproject/shamir"
	"github.com/renproject/shamir/curve"
	stu "github.com/renproject/shamir/testutil"
	mtu "github.com/renproject/mpc/testutil"
)

// RandomValidSharing creates a random and valid sharing for the indices with
// reconstruction threshold k and Pedersen parameter h.
func RandomValidSharing(indices []secp256k1.Secp256k1N, k int, h curve.Point) brng.Sharing {
	shares := make(shamir.VerifiableShares, len(indices))
	commitment := shamir.NewCommitmentWithCapacity(k)
	vssharer := shamir.NewVSSharer(indices, h)
	vssharer.Share(&shares, &commitment, secp256k1.RandomSecp256k1N(), k)

	return brng.NewSharing(shares, commitment)
}

// RandomInvalidSharing creates a random sharing with a fault for the share
// corresponding to player indices[badIndex].
func RandomInvalidSharing(
	indices []secp256k1.Secp256k1N,
	k int,
	h curve.Point,
	badIndex int,
) brng.Sharing {
	shares := make(shamir.VerifiableShares, len(indices))
	commitment := shamir.NewCommitmentWithCapacity(k)
	vssharer := shamir.NewVSSharer(indices, h)
	vssharer.Share(&shares, &commitment, secp256k1.RandomSecp256k1N(), k)

	// Perturb the bad indice.
	perturbShare(&shares[badIndex])

	return brng.NewSharing(shares, commitment)
}

// RandomValidRow constructs a random row for the players with the given
// indices with batch size b from sharings with reconstruction threshold k and
// Pedersen parameter h.
func RandomValidRow(indices []secp256k1.Secp256k1N, k, b int, h curve.Point) brng.Row {
	row := make(brng.Row, b)
	for i := range row {
		row[i] = RandomValidSharing(indices, k, h)
	}
	return row
}

// RandomInvalidRow constructs a random row with faults in the batches given by
// badBatches and for the player with index indices[badIndex].
func RandomInvalidRow(
	indices []secp256k1.Secp256k1N,
	k, b int,
	h curve.Point,
	badIndex int,
	badBatches []int,
) brng.Row {
	row := make(brng.Row, b)

	j := 0
	for i := range row {
		if j < len(badBatches) && i == badBatches[j] {
			row[i] = RandomInvalidSharing(indices, k, h, badIndex)
			j++
		} else {
			row[i] = RandomValidSharing(indices, k, h)
		}
	}

	return row
}

// RandomValidTable contructs a random valid table for the players with the
// given indices with t rows that have a batch size b, reconstruction threshold
// k and Pedersen parameter h.
func RandomValidTable(indices []secp256k1.Secp256k1N, h curve.Point, k, b, t int) brng.Table {
	table := make(brng.Table, t)
	for i := range table {
		table[i] = RandomValidRow(indices, k, b, h)
	}
	return table
}

// RandomInvalidTable constructs a random table with faults in the slice
// corresponding to player indices[badIndex].
func RandomInvalidTable(
	indices []secp256k1.Secp256k1N,
	h curve.Point,
	n, k, b, t, badIndex int,
) (brng.Table, map[int][]int) {
	table := make(brng.Table, n)
	badIndices := randomIndices(t, 1)
	faultLocations := make(map[int][]int)

	j := 0
	for i := range table {
		if j < len(badIndices) && i == badIndices[j] {
			badBatches := randomIndices(b, 1)
			faultLocations[badIndices[j]] = badBatches
			table[i] = RandomInvalidRow(indices, k, b, h, badIndex, badBatches)
			j++
		} else {
			table[i] = RandomValidRow(indices, k, b, h)
		}
	}

	return table, faultLocations
}

// RandomValidSlice constructs a random valid slice for the player with index
// to, where the reconstruction threshold is k, the batch size is b, the height
// of the columns is t and h is the Pedersen parameter.
func RandomValidSlice(
	to secp256k1.Secp256k1N,
	indices []secp256k1.Secp256k1N,
	h curve.Point,
	k, b, t int,
) brng.Slice {
	table := RandomValidTable(indices, h, k, b, t)
	slice := table.Slice(to, indices)
	return slice
}

// RandomInvalidSlice constructs a random slice with some faults, and returns
// the slice as well as a list of the faults.
func RandomInvalidSlice(
	to secp256k1.Secp256k1N,
	indices []secp256k1.Secp256k1N,
	h curve.Point,
	n, k, b, t int,
) (brng.Slice, []brng.Element) {
	badIndex := -1
	for i, index := range indices {
		if index.Eq(&to) {
			badIndex = i
		}
	}
	if badIndex == -1 {
		panic("to index was not found in indices")
	}

	table, faultLocations := RandomInvalidTable(indices, h, n, k, b, t, badIndex)
	slice := table.Slice(to, indices)

	var faults []brng.Element

	for player, batches := range faultLocations {
		for _, batch := range batches {
			var fault brng.Element
			fault.Set(slice[batch][player])
			faults = append(faults, fault)
		}
	}

	return slice, faults
}

// RowIsValid returns true if all of the sharings in the given row are valid
// with respect to the commitments and the shares form a consistent k-sharing.
func RowIsValid(row brng.Row, k int, indices []secp256k1.Secp256k1N, h curve.Point) bool {
	reconstructor := shamir.NewReconstructor(indices)
	checker := shamir.NewVSSChecker(h)

	for _, sharing := range row {
		c := sharing.Commitment()
		for _, share := range sharing.Shares() {
			if !checker.IsValid(&c, &share) {
				return false
			}
		}

		if !stu.VsharesAreConsistent(sharing.Shares(), &reconstructor, k) {
			return false
		}
	}

	return true
}

func randomIndices(n, k int) []int {
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}

	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})
	ret := indices[:k]

	sort.Ints(ret)
	return ret
}

func perturbShare(share *shamir.VerifiableShare) {
	r := rand.Intn(3)
	switch r {
	case 0:
		stu.PerturbValue(share)
	case 1:
		stu.PerturbDecommitment(share)
	case 2:
		stu.PerturbIndex(share)
	default:
		panic("invalid case")
	}
}

// ToSlice returns a slice of the table at index `to`
func TableToSlice(table brng.Table, to mtu.ID) brng.Slice {
	if len(table) == 0 {
		return nil
	}

	imax := table[0].N() 			// from direction
	jmax := table[0].N() 			// to direction
	kmax := table.BatchSize() // batch direction

	slice := make(brng.Slice, kmax)

	for k := 0; k < kmax; k++ {
		col := make(brng.Col, imax)

		for i := 0; i < imax; i++ {

			for j := 0; j < jmax; j++ {
				if mtu.ID(j) != to {
					continue
				}

				var commitment shamir.Commitment
				sharing    := table[i][k]
				shares     := sharing.Shares()
				from       := secp256k1.NewSecp256k1N(uint64(i))
				commitment.Set(sharing.Commitment())

				element    := brng.NewElement(from, shares[j], commitment)

				col = append(col, element)
			}
		}

		slice[k] = col
	}

	return slice
}
