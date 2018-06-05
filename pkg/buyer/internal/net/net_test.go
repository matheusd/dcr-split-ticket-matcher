package net

import (
	"testing"
)

func TestIsSubdomain(t *testing.T) {
	type testCase struct {
		root string
		sub  string
		res  bool
	}

	var tests []testCase
	tests = []testCase{
		{"example.com", "example.com", true},
		{"example.com", "example.cot", false},
		{"example.com", "dummy.example.com", true},
		{"notexample.com", "dummy.example.com", false},
		{"example.co.uk", "dummy.example.co.uk", true},
	}

	for _, tc := range tests {
		res := IsSubDomain(tc.root, tc.sub)
		if res != tc.res {
			t.Errorf("case %s vs %s: expected %v found %v",
				tc.root, tc.sub, tc.res, res)
		}
	}
}
