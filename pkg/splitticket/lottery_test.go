package splitticket

import (
	"encoding/hex"
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

func TestSecretNumberHashes(t *testing.T) {
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
