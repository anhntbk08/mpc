package compute_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/renproject/mpc/rng/compute"
	"github.com/renproject/secp256k1-go"

	"github.com/renproject/shamir"
	"github.com/renproject/shamir/curve"
)

var _ = Describe("RNG computation helper functions", func() {
	trials := 50
	k := 5

	randomiseCommitment := func(com *shamir.Commitment) {
		// This effectively sets the length of the commitment to 0, so that the
		// next append will add an element at index 0.
		com.Set(shamir.Commitment{})

		for i := 0; i < k; i++ {
			com.AppendPoint(curve.Random())
		}
	}

	polyEval := func(x secp256k1.Secp256k1N, coeffs []secp256k1.Secp256k1N) secp256k1.Secp256k1N {
		acc := coeffs[len(coeffs)-1]

		for i := len(coeffs) - 2; i >= 0; i-- {
			acc.Mul(&acc, &x)
			acc.Add(&acc, &coeffs[i])
			acc.Normalize()
		}

		return acc
	}

	Specify("output commitments should be computed correctly", func() {
		coms := make([]shamir.Commitment, k)

		for i := range coms {
			coms[i] = shamir.NewCommitmentWithCapacity(k)
		}

		for i := 0; i < trials; i++ {
			for j := range coms {
				randomiseCommitment(&coms[j])
			}

			output := OutputCommitment(coms)

			for j := 0; j < output.Len(); j++ {
				actual := output.GetPoint(j)
				expected := coms[j].GetPoint(0)
				Expect(actual.Eq(&expected)).To(BeTrue())
			}
		}
	})

	Specify("commitments for shares should be computed correctly", func() {
		var index secp256k1.Secp256k1N
		var bs [32]byte

		coeffs := make([][]secp256k1.Secp256k1N, k)
		for i := range coeffs {
			coeffs[i] = make([]secp256k1.Secp256k1N, k)
		}

		points := make([][]curve.Point, k)
		for i := range points {
			points[i] = make([]curve.Point, k)
		}

		coms := make([]shamir.Commitment, k)
		for i := range coms {
			coms[i] = shamir.NewCommitmentWithCapacity(k)
		}

		for i := 0; i < trials; i++ {
			index = secp256k1.RandomSecp256k1N()

			for j := range coeffs {
				for l := range coeffs[j] {
					coeffs[j][l] = secp256k1.RandomSecp256k1N()
					coeffs[j][l].GetB32(bs[:])
					points[j][l].BaseExp(bs)
				}
			}

			for j := range coms {
				coms[j].Set(shamir.Commitment{})
				for l := range points[j] {
					coms[j].AppendPoint(points[l][j])
				}
			}

			output := ShareCommitment(index, coms)

			expected := curve.New()
			for j := 0; j < output.Len(); j++ {
				y := polyEval(index, coeffs[j])
				y.GetB32(bs[:])

				actual := output.GetPoint(j)
				expected.BaseExp(bs)

				Expect(actual.Eq(&expected)).To(BeTrue())
			}
		}
	})

	Specify("shares of shares should be computed correctly", func() {
		var to, from secp256k1.Secp256k1N

		values := make([]secp256k1.Secp256k1N, k)
		decoms := make([]secp256k1.Secp256k1N, k)
		vshares := make(shamir.VerifiableShares, k)

		for i := 0; i < trials; i++ {
			to = secp256k1.RandomSecp256k1N()
			from = secp256k1.RandomSecp256k1N()

			for j := 0; j < k; j++ {
				values[j] = secp256k1.RandomSecp256k1N()
				decoms[j] = secp256k1.RandomSecp256k1N()
			}

			for j := range vshares {
				vshares[j] = shamir.NewVerifiableShare(
					shamir.NewShare(from, values[j]),
					decoms[j],
				)
			}

			output := ShareOfShare(to, vshares)

			// The value of the share should be correct.
			share := output.Share()
			actual := share.Value()
			expected := polyEval(to, values)

			Expect(actual.Eq(&expected)).To(BeTrue())

			// The decommitment of the share should be correct.
			actual = output.Decommitment()
			expected = polyEval(to, decoms)

			Expect(actual.Eq(&expected)).To(BeTrue())
		}
	})
})
