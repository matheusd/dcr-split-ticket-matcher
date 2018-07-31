package splitticket

import (
	"encoding/hex"
	"math"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
)

func chainHashFromStr(str string) *chainhash.Hash {
	res := new(chainhash.Hash)
	err := chainhash.Decode(res, str)
	if err != nil {
		panic(err)
	}
	return res
}

func addrFromStr(str string) dcrutil.Address {
	addr, err := dcrutil.DecodeAddress(str)
	if err != nil {
		panic(err)
	}
	return addr
}

func TestSelectContributionAmount(t *testing.T) {
	t.Parallel()

	var err error

	// syntatic sugar to help out making the test cases
	mka := func(args ...dcrutil.Amount) []dcrutil.Amount {
		return args
	}

	errTests := []struct {
		ticketPrice  dcrutil.Amount
		partFee      dcrutil.Amount
		totalPoolFee dcrutil.Amount
		amounts      []dcrutil.Amount
	}{
		// test unordered contribution amounts
		{30, 1, 1, mka(20, 10)},

		// test contributions < ticket price
		{60, 1, 1, mka(10, 20)},

		// test contributions < ticket price + pool/part fee
		{30, 1, 1, mka(10, 20)},
		{30, 1, 1, mka(11, 20)},

		// test participant with < minimum
		{30, 11, 1, mka(10, 20)},

		// test negative amount
		{30, 1, 1, mka(-10, 50)},
	}

	for i, tc := range errTests {
		_, _, err = SelectContributionAmounts(tc.amounts, tc.ticketPrice,
			tc.partFee, tc.totalPoolFee)
		if err == nil {
			t.Errorf("error test case %d should have returned an error", i)
		}
	}

	tests := []struct {
		ticketPrice  dcrutil.Amount
		partFee      dcrutil.Amount
		totalPoolFee dcrutil.Amount
		amounts      []dcrutil.Amount
		res          []dcrutil.Amount
		pres         []dcrutil.Amount
	}{
		// Test when everyone the split can happen equally
		{1000, 10, 40, mka(2000, 2000, 2000, 2000), mka(240, 240, 240, 240), mka(10, 10, 10, 10)},

		// Test the very minimum needed for a split, given a ticket price and fees
		{1000, 10, 40, mka(260, 260, 260, 260), mka(240, 240, 240, 240), mka(10, 10, 10, 10)},

		// Test when the division can't be exact
		{1000, 10, 30, mka(1000, 1000, 1000), mka(323, 323, 324), mka(11, 11, 8)},

		// Test possible division problems
		{996, 10, 30, mka(1000, 1000, 1000), mka(322, 322, 322), mka(10, 10, 10)},

		// Test various divisions when one or more contributors can't provide as
		// much as some others
		{1000, 10, 30, mka(100, 1000, 1000), mka(87, 441, 442), mka(3, 14, 13)},
		{1000, 10, 40, mka(100, 1000, 1000, 1000), mka(86, 291, 291, 292), mka(4, 13, 13, 10)},
		{1000, 10, 40, mka(100, 100, 1000, 1000), mka(86, 86, 393, 395), mka(4, 4, 17, 15)},
		{1000, 10, 40, mka(100, 100, 420, 420), mka(86, 86, 393, 395), mka(4, 4, 17, 15)},
		{1000, 10, 50, mka(100, 100, 500, 500, 500), mka(85, 85, 260, 260, 260), mka(5, 5, 14, 14, 12)},
		{1000, 10, 50, mka(40, 50, 60, 70, 1000), mka(28, 38, 47, 57, 780), mka(2, 2, 3, 3, 40)},
		{1000, 10, 50, mka(200, 200, 200, 200, 500), mka(180, 180, 180, 180, 230), mka(10, 10, 10, 10, 10)},
		{1000, 10, 50, mka(200, 200, 200, 201, 500), mka(180, 180, 180, 181, 229), mka(10, 10, 10, 10, 10)},
	}

	for i, tc := range tests {
		res, pres, err := SelectContributionAmounts(tc.amounts, tc.ticketPrice,
			tc.partFee, tc.totalPoolFee)
		if err != nil {
			t.Errorf("unexpected error in case %d: %v", i, err)
			continue
		}

		if len(res) != len(tc.res) {
			t.Errorf("returned different number of items (%d) than expected (%d)",
				len(res), len(tc.res))
		}

		responseSum := dcrutil.Amount(0)
		responsePoolFeeSum := dcrutil.Amount(0)
		for j, v := range res {
			if v != tc.res[j] {
				t.Errorf("response element %d (%s) of test case %d different "+
					"than expected (%s)", j, v, i, tc.res[j])
			}
			if pres[j] != tc.pres[j] {
				t.Errorf("response pool fee element %d (%s) of test case %d "+
					"different than expected (%s)", j, pres[j], i, tc.pres[j])
			}
			responseSum += v + pres[j]
			responsePoolFeeSum += pres[j]
		}

		if responseSum != tc.ticketPrice {
			t.Errorf("sum of response elements + pool fee (%s) different "+
				"than expected (%s)", responseSum, tc.ticketPrice)
		}

		if responsePoolFeeSum != tc.totalPoolFee {
			t.Errorf("sum of response pool fee elements (%s) different than "+
				"expected (%s)", responsePoolFeeSum, tc.totalPoolFee)
		}
	}
}

func TestSecretNumberHashes(t *testing.T) {
	t.Parallel()

	mainchainHash := new(chainhash.Hash)
	chainhash.Decode(mainchainHash, "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9")

	tests := []struct {
		nb       uint64
		expected string
	}{
		{0, "09504f08215f8b5b0b834a82905ae08d27445715790d55dc2ef513bee4dcdf9f"},
		{1, "f25ec2704cf7811b162c005bd2b0bb81f2450a9112f1a8753e63bb2c3e6287e9"},
		{21e15, "0f360757ac8f6332dab5ae700f33e8ec7132e215a439fa256296bdd1766dcbb6"},
	}

	var expected SecretNumberHash

	for _, tc := range tests {
		nb := SecretNumber(tc.nb)
		res := nb.Hash(mainchainHash)
		decoded, _ := hex.DecodeString(tc.expected)
		copy(expected[:], decoded[:])
		if !res.Equals(SecretNumberHash(expected)) {
			t.Errorf("invalid hash test vector when testing number %d", tc.nb)
		}
	}
}

func TestLotteryCommitment(t *testing.T) {
	t.Parallel()

	chainHashes := []*chainhash.Hash{
		chainHashFromStr("000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"),
		chainHashFromStr("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"),
	}

	addresses := []dcrutil.Address{
		addrFromStr("TsVP6uM4FDmVWpues3HH7PWGFUKVQzECi9v"),
		addrFromStr("TcqLUGXsEjb9TvvEA5knLi1oT8dDRofDQHB"),
		addrFromStr("TScNA7C7gHPFHpfNH3KmvToCSynjTmHQSt8"),
		addrFromStr("TsfPDKyDUbYyqwfNWsZpZqxfe9AeDzapkrR"),
	}

	tests := []struct {
		hashIndex int
		addrs     []int
		nbs       []SecretNumber
		amounts   []dcrutil.Amount
		res       string
	}{
		{0, []int{0, 1, 2}, []SecretNumber{0, 0, 0}, []dcrutil.Amount{10, 20, 30}, "d20fe4fb24e16d7030532574c252d5c41d8a291ffb3972c2545f5e89ab41ecfc"},
		{0, []int{0, 1, 2}, []SecretNumber{0, 0, 0}, []dcrutil.Amount{10, 20, 30}, "d20fe4fb24e16d7030532574c252d5c41d8a291ffb3972c2545f5e89ab41ecfc"},
		{1, []int{0, 1, 2}, []SecretNumber{0, 0, 0}, []dcrutil.Amount{10, 20, 30}, "6ab2adbbb2ac661bb8f52f9932630602946d638bcf04ed3c299b040ca6a47930"},
		{0, []int{0, 1, 3}, []SecretNumber{0, 0, 0}, []dcrutil.Amount{10, 20, 30}, "0bb5cb6372cda797932f44527da3d29920977af003781e7c09a4ea400d5d99b4"},
		{0, []int{0, 1, 2}, []SecretNumber{0, 0, 1}, []dcrutil.Amount{10, 20, 30}, "0bf2aae5036d2fb9742d456d466d078b73c2fd27455bbeb0d2869ba1e2b2735b"},
		{0, []int{0, 1, 2}, []SecretNumber{0, 0, 0}, []dcrutil.Amount{11, 20, 30}, "7c1dd2b28cadcb58b9a3f62c3bd3f5013c31d2ce5f26512f79667a2edd894775"},
	}

	for i, tc := range tests {
		chainHash := chainHashes[tc.hashIndex]

		hashes := make([]SecretNumberHash, len(tc.nbs))
		for j, nb := range tc.nbs {
			hashes[j] = nb.Hash(chainHash)
		}

		addrs := make([]dcrutil.Address, len(tc.addrs))
		for j, idxAddr := range tc.addrs {
			addrs[j] = addresses[idxAddr]
		}

		res := CalcLotteryCommitmentHash(hashes, tc.amounts, addrs, chainHash)

		resStr := hex.EncodeToString(res[:])
		if resStr != tc.res {
			t.Errorf("different hash (%s) than expected in test case %d", resStr, i)
		}
	}
}

func TestLotteryResults(t *testing.T) {
	t.Parallel()

	chainHashes := []*chainhash.Hash{
		chainHashFromStr("000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"),
		chainHashFromStr("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"),
	}

	hi := ^SecretNumber(0)
	hiamt := dcrutil.Amount(21e14)

	tests := []struct {
		nbs       []SecretNumber
		amounts   []dcrutil.Amount
		hashIndex int
		resCoin   dcrutil.Amount
		resIndex  int
	}{
		{[]SecretNumber{0, 0, 0}, []dcrutil.Amount{10, 20, 30}, 0, 4, 0},
		{[]SecretNumber{0, 0, 0}, []dcrutil.Amount{10, 20, 30}, 1, 10, 1},
		{[]SecretNumber{0, 0, 1}, []dcrutil.Amount{10, 20, 30}, 0, 10, 1},
		{[]SecretNumber{0, 0, 1}, []dcrutil.Amount{10, 20, 30}, 1, 47, 2},
		{[]SecretNumber{1, 0, 0}, []dcrutil.Amount{10, 20, 30}, 0, 17, 1},
		{[]SecretNumber{0, 0, 13}, []dcrutil.Amount{10, 20, 30}, 0, 0, 0},
		{[]SecretNumber{0, 0, 20}, []dcrutil.Amount{10, 20, 30}, 0, 59, 2},
		{[]SecretNumber{1, 2, 3}, []dcrutil.Amount{10, 20, 30}, 0, 31, 2},
		{[]SecretNumber{1, 2, 4}, []dcrutil.Amount{10, 20, 30}, 0, 56, 2},
		{[]SecretNumber{1, 3, 4}, []dcrutil.Amount{10, 20, 30}, 0, 25, 1},
		{[]SecretNumber{1, 3, 4}, []dcrutil.Amount{30, 10, 20}, 0, 25, 0},
		{[]SecretNumber{1, 2, 3}, []dcrutil.Amount{10, 20, 30}, 1, 49, 2},
		{[]SecretNumber{hi, hi - 1, hi - 2}, []dcrutil.Amount{10, 20, 30}, 1, 14, 1},
		{[]SecretNumber{hi, hi - 1, hi - 3}, []dcrutil.Amount{10, 20, 30}, 1, 29, 1},
		{[]SecretNumber{1, 2, 3}, []dcrutil.Amount{hiamt / 3, hiamt / 3, hiamt / 3}, 0, 577681244570491, 0},
		{[]SecretNumber{1, 2, 4}, []dcrutil.Amount{hiamt/3 - 1, hiamt / 3, hiamt / 3}, 0, 1424220813911263, 2},
	}

	for _, tc := range tests {
		coin, index := CalcLotteryResult(tc.nbs, tc.amounts, chainHashes[tc.hashIndex])
		if coin != tc.resCoin {
			t.Errorf("different coin (%s) than expected (%s)", coin, tc.resCoin)
		} else if index != tc.resIndex {
			t.Errorf("different index (%d) than expected (%d)", index, tc.resIndex)
		}
	}
}

func TestLotteryResultsStatistics(t *testing.T) {
	t.Parallel()

	chainHashes := []*chainhash.Hash{
		chainHashFromStr("000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"),
		chainHashFromStr("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"),
	}

	var amounts []dcrutil.Amount
	var i int
	var nbs []SecretNumber

	amounts = []dcrutil.Amount{10000, 20000, 30000}
	totalAmount := dcrutil.Amount(60000)
	totalRepeats := 20
	totalTries := int(totalAmount) * totalRepeats // sum(amounts) * 10 (on average this should be sufficient to find a collision mod sum(amount))
	expectedCounts := make([]int, len(amounts))
	for i, amt := range amounts {
		// we expect, given `totalTries` tickets, that a voter with the given
		// % of a full ticket be selected %*totalTries times (on average; there
		// will be some variation due to randomness)
		partPerc := float64(amt) / float64(totalAmount)
		expectedCounts[i] = int(math.Floor(partPerc * float64(totalTries)))
	}

	for _, chainHash := range chainHashes {

		// calculate a resultset that is likely to include all coin indexes between
		// 0..totalAmount-1 to perform some statistics in it.
		nbs = []SecretNumber{0, 0, 0}

		// histogram of coin and voter index selection
		byCoin := make(map[dcrutil.Amount]uint32, int(totalAmount))
		byIndex := make(map[int]uint32, len(amounts))

		for i = 0; i < int(totalTries); i++ {
			nbs[0] = SecretNumber(i)
			coin, index := CalcLotteryResult(nbs, amounts, chainHash)

			if coin >= totalAmount {
				t.Fatalf("lottery selected coin (%s) >= total amount (%s)",
					coin, totalAmount)
			}

			if coin < 0 {
				t.Fatalf("lottery selected negative coin (%s)", coin)
			}

			if index >= len(amounts) {
				t.Fatalf("lottery selected voter index (%d) >= available (%d)",
					index, len(amounts))
			}

			if index < 0 {
				t.Fatalf("lottery selected negative index (%d)", index)
			}

			byCoin[coin]++
			byIndex[index]++
		}

		if len(byIndex) < len(amounts) {
			t.Fatal("some voters where never selected")
		}

		if len(byCoin) < int(totalAmount) {
			t.Fatal("some coins where never selected")
		}

		maxVariancePerc := 0.25 / 100.0 // 0.25% of maximum variance from expected allowed
		for i, expected := range expectedCounts {
			min := uint32(math.Floor(float64(expected) * (1 - maxVariancePerc)))
			max := uint32(math.Ceil(float64(expected) * (1 + maxVariancePerc)))
			if byIndex[i] < min {
				t.Errorf("found less selections (%d) than minimum expected (%d) "+
					"for voter %d", byIndex[i], min, i)
			} else if byIndex[i] > max {
				t.Errorf("found more selections (%d) than maximum expected (%d) "+
					"for voter %d", byIndex[i], max, i)
			}
		}
	}

}
