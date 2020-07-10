package rng_test

import (
	"bytes"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/renproject/shamir"
	"github.com/renproject/shamir/curve"
	"github.com/renproject/shamir/shamirutil"
	"github.com/renproject/surge"

	"github.com/renproject/mpc/mpcutil"
	"github.com/renproject/mpc/open"
	"github.com/renproject/mpc/rng"
	"github.com/renproject/mpc/rng/rngutil"
)

var _ = Describe("RNG", func() {
	rand.Seed(int64(time.Now().Nanosecond()))

	Describe("RNG Properties", func() {
		var b, k int
		var indices []open.Fn
		var index open.Fn
		var h curve.Point
		var isZero bool

		// Setup is run before every test. It randomises the test parameters
		Setup := func() (
			[]open.Fn,
			open.Fn,
			int,
			int,
			curve.Point,
			bool,
		) {
			// n is the number of players participating in the RNG protocol
			// n ∈ [5, 10]
			n := 5 + rand.Intn(6)

			// indices represent the list of index for each player
			// They are Secp256k1N representations of sequential n values
			indices := shamirutil.RandomIndices(n)

			// index denotes the current player's index
			// This is a randomly chosen index from indices
			index := indices[rand.Intn(len(indices))]

			// b is the total number of random numbers to be generated in one
			// execution of RNG protocol, i.e. the batch number
			b := 3 + rand.Intn(3)

			// k is the threshold for random number generation, or the minimum
			// number of shares required to reconstruct the secret in the
			// secret sharing scheme. Based on our BRNG to RNG scheme, k is
			// also the number of times BRNG needs to be run before using their
			// outputs to generate an unbiased random number
			k := 3 + rand.Intn(n-3)

			// h is the elliptic curve point, used as the Pedersen Commitment
			// Scheme Parameter
			h := curve.Random()

			return indices, index, b, k, h, false
		}

		BeforeEach(func() {
			indices, index, b, k, h, isZero = Setup()
		})

		Context("State Transitions and Events", func() {
			Context("When in Init state", func() {
				Specify("Supply valid BRNG shares/commitments when k = 1", func() {
					k = 1
					setsOfShares, setsOfCommitments := rngutil.BRNGOutputBatch(index, b, k, h)
					_, rnger := rng.New(index, indices, uint32(b), uint32(k), h)
					event := rnger.TransitionShares(setsOfShares, setsOfCommitments, isZero)

					Expect(event).To(Equal(rng.RNGsReconstructed))
					Expect(rnger.State()).To(Equal(rng.Done))
					Expect(rnger.HasConstructedShares()).To(BeTrue())
				})

				Specify("Special scenario when k = 1", func() {
					// If an RNG machine in the Init state is supplied with
					// valid sets of shares and commitments from its own BRNG
					// outputs it transitions to the WaitingOpen state.  But if
					// the reconstruction threshold is k = 1, then in that
					// trivial case, a single machine can construct the entire
					// secret just by itself.  This should not be the scenario
					// ideally, but we will cover it nonetheless
					setsOfShares, setsOfCommitments := rngutil.BRNGOutputBatch(index, b, 1, h)

					// Once we have `b` sets of shares and commitments we are
					// ready to transition the RNG machine
					_, rnger := rng.New(index, indices, uint32(b), uint32(1), h)
					event := rnger.TransitionShares(setsOfShares, setsOfCommitments, isZero)

					Expect(event).To(Equal(rng.RNGsReconstructed))
					Expect(rnger.State()).To(Equal(rng.Done))
					Expect(rnger.HasConstructedShares()).To(BeTrue())
				})
			})
		})

		Context("Marshaling and Unmarshaling", func() {
			var rnger rng.RNGer
			var openingsByPlayer map[open.Fn]shamir.VerifiableShares
			var ownSetsOfShares []shamir.VerifiableShares
			var ownSetsOfCommitments [][]shamir.Commitment

			JustBeforeEach(func() {
				_, rnger = rng.New(index, indices, uint32(b), uint32(k), h)
				ownSetsOfShares, ownSetsOfCommitments, openingsByPlayer, _ =
					rngutil.RNGSharesBatch(indices, index, b, k, h, isZero)

				rnger.TransitionShares(ownSetsOfShares, ownSetsOfCommitments, isZero)
			})

			It("Should correctly marshal and unmarshal (WaitingOpen)", func() {
				buf := bytes.NewBuffer([]byte{})

				m, err := rnger.Marshal(buf, rnger.SizeHint())
				Expect(err).ToNot(HaveOccurred())
				Expect(m).To(Equal(0))

				var rnger2 rng.RNGer
				m, err = rnger2.Unmarshal(buf, rnger.SizeHint())
				Expect(err).ToNot(HaveOccurred())
				Expect(m).To(Equal(0))

				Expect(rnger.BatchSize()).To(Equal(rnger2.BatchSize()))
				Expect(rnger.State()).To(Equal(rnger2.State()))
				Expect(rnger.N()).To(Equal(rnger2.N()))
				Expect(rnger.Threshold()).To(Equal(rnger2.Threshold()))
				Expect(rnger.ReconstructedShares()).To(Equal(rnger2.ReconstructedShares()))

				for _, j := range indices {
					expectedShares := rnger.DirectedOpenings(j)
					shares := rnger2.DirectedOpenings(j)

					Expect(expectedShares).To(Equal(shares))
				}
			})

			It("should correctly marshal and unmarshal (Done)", func() {
				count := 1
				for _, from := range indices {
					if count == k {
						break
					}

					_ = rnger.TransitionOpen(openingsByPlayer[from])
				}
				Expect(rnger.State()).To(Equal(rng.Done))
				Expect(len(rnger.ReconstructedShares())).To(Equal(b))

				buf := bytes.NewBuffer([]byte{})

				m, err := rnger.Marshal(buf, rnger.SizeHint())
				Expect(err).ToNot(HaveOccurred())
				Expect(m).To(Equal(0))

				var rnger2 rng.RNGer
				m, err = rnger2.Unmarshal(buf, rnger.SizeHint())
				Expect(err).ToNot(HaveOccurred())
				Expect(m).To(Equal(0))

				Expect(rnger.BatchSize()).To(Equal(rnger2.BatchSize()))
				Expect(rnger.State()).To(Equal(rnger2.State()))
				Expect(rnger.N()).To(Equal(rnger2.N()))
				Expect(rnger.Threshold()).To(Equal(rnger2.Threshold()))
				Expect(rnger.ReconstructedShares()).To(Equal(rnger2.ReconstructedShares()))

				for _, j := range indices {
					expectedShares := rnger.DirectedOpenings(j)
					shares := rnger2.DirectedOpenings(j)

					Expect(expectedShares).To(Equal(shares))
				}
			})

			It("Should fail when marshaling with not enough bytes", func() {
				buf := bytes.NewBuffer([]byte{})

				for i := 0; i < rnger.SizeHint(); i++ {
					buf.Reset()
					_, err := rnger.Marshal(buf, i)
					Expect(err).To(HaveOccurred())
				}
			})

			It("Should fail when unmarshaling with not enough bytes", func() {
				bs, _ := surge.ToBinary(rnger)

				var rnger2 rng.RNGer
				for i := 0; i < rnger.SizeHint(); i++ {
					buf := bytes.NewBuffer(bs)

					_, err := rnger2.Unmarshal(buf, i)
					Expect(err).To(HaveOccurred())
				}
			})
		})
	})

	Describe("Network Simulation", func() {
		var n, b, k, nOffline int
		var indices []open.Fn
		var h curve.Point
		var isZero bool
		var ids []mpcutil.ID
		var setsOfSharesByPlayer map[open.Fn][]shamir.VerifiableShares
		var setsOfCommitmentsByPlayer [][]shamir.Commitment
		var shuffleMsgs func([]mpcutil.Message)
		var isOffline map[mpcutil.ID]bool
		var machines []mpcutil.Machine

		CheckMachines := func(
			machines []mpcutil.Machine,
			isOffline map[mpcutil.ID]bool,
			b, k int,
			h curve.Point,
		) {
			// ID of the first online machine
			i := 0
			for isOffline[machines[i].ID()] {
				i = i + 1
			}

			// Get the unbiased random numbers calculated by that RNG machine
			referenceRNShares := machines[i].(*rngutil.RngMachine).RandomNumbersShares()
			referenceCommitments := machines[i].(*rngutil.RngMachine).Commitments()

			vssChecker := shamir.NewVSSChecker(h)

			for j := i + 1; j < len(machines); j++ {
				// Ignore if that machine is offline
				if isOffline[machines[j].ID()] {
					continue
				}

				rnShares := machines[j].(*rngutil.RngMachine).RandomNumbersShares()
				Expect(len(referenceRNShares)).To(Equal(len(rnShares)))

				// Every player has computed the same commitments for the batch
				// of unbiased random numbers
				comms := machines[j].(*rngutil.RngMachine).Commitments()
				for l, c := range comms {
					Expect(c.Eq(&referenceCommitments[l])).To(BeTrue())
				}

				// Verify that each machine's share of the unbiased random
				// number (for all batches) are valid with respect to the
				// reference commitments
				for l, vshare := range rnShares {
					Expect(vssChecker.IsValid(&comms[l], &vshare)).To(BeTrue())
				}
			}

			// Form the indices for machines that were online and a
			// reconstructor for those indices
			onlineIndices := make([]open.Fn, 0, len(machines))
			for j := 0; j < len(machines); j++ {
				if isOffline[machines[j].ID()] {
					continue
				}
				evaluationPoint := machines[j].(*rngutil.RngMachine).Index()
				onlineIndices = append(onlineIndices, evaluationPoint)
			}
			reconstructor := shamir.NewReconstructor(onlineIndices)

			// For every batch in batch size, the shares that every player has
			// should be consistent
			for i := 0; i < b; i++ {
				shares := make(shamir.Shares, 0, len(machines))

				for j := 0; j < len(machines); j++ {
					if isOffline[machines[j].ID()] {
						continue
					}

					vshare := machines[j].(*rngutil.RngMachine).RandomNumbersShares()[i]

					shares = append(shares, vshare.Share())
				}

				Expect(shamirutil.SharesAreConsistent(shares, &reconstructor, k-1)).ToNot(BeTrue())
				Expect(shamirutil.SharesAreConsistent(shares, &reconstructor, k)).To(BeTrue())
			}
		}

		Setup := func() {
			// Randomise RNG network scenario
			n = 15 + rand.Intn(6)
			indices = shamirutil.SequentialIndices(n)
			b = 3 + rand.Intn(3)
			k = rngutil.Min(3+rand.Intn(n-3), 7)
			h = curve.Random()
			isZero = false

			// Machines (players) participating in the RNG protocol
			ids = make([]mpcutil.ID, n)

			// Get BRNG outputs for all players
			setsOfSharesByPlayer, setsOfCommitmentsByPlayer =
				rngutil.BRNGOutputFullBatch(indices, b, k, k, h)

			// Append machine IDs and get offline machines
			for i := range indices {
				id := mpcutil.ID(i)
				ids[i] = id
			}
			nOffline = rand.Intn(n - k + 1)
			shuffleMsgs, isOffline = mpcutil.MessageShufflerDropper(ids, nOffline)
		}

		MakeMachines := func() {
			machines = make([]mpcutil.Machine, n)
			for i, index := range indices {
				rngMachine := rngutil.NewRngMachine(
					mpcutil.ID(i), index, indices, b, k, h, isZero,
					setsOfSharesByPlayer[index],
					setsOfCommitmentsByPlayer,
				)
				machines[i] = &rngMachine
			}
		}

		BeforeEach(func() {
			Setup()
		})

		Specify("RNG machines should reconstruct the consistent shares for random numbers", func() {
			MakeMachines()
			network := mpcutil.NewNetwork(machines, shuffleMsgs)
			network.SetCaptureHist(true)

			err := network.Run()
			Expect(err).ToNot(HaveOccurred())

			CheckMachines(machines, isOffline, b, k, h)
		})

		Specify("With not all RNG machines contributing their BRNG shares", func() {
			// Mark some machines as being idle specifically, at the most k+1
			// should not be idle so (n - nOffline) - k - 1 should be idle
			// because only (n - nOffline) machines are online
			idleCount := 0
			for j, index := range indices {
				if isOffline[mpcutil.ID(j)] {
					continue
				}
				if idleCount == rngutil.Max(0, (n-nOffline)-k-1) {
					break
				}

				setsOfSharesByPlayer[index] = []shamir.VerifiableShares{}
				idleCount++
			}

			MakeMachines()
			network := mpcutil.NewNetwork(machines, shuffleMsgs)
			network.SetCaptureHist(true)

			err := network.Run()
			Expect(err).ToNot(HaveOccurred())

			CheckMachines(machines, isOffline, b, k, h)
		})
	})
})
