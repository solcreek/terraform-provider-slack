// SPDX-License-Identifier: MPL-2.0
package provider

import "testing"

func TestJSONEquality(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		equal bool
	}{
		{`{"a":1,"b":false}`, `{ "b":false,"a":1 }`, true},
		{`{"a":false}`, `{}`, false},
		{`{"a":[1,2]}`, `{"a":[2,1]}`, false},
		{`{"a":9007199254740992}`, `{"a":9007199254740993}`, false},
		{`null`, `null`, false},
		{`{} {}`, `{}`, false},
	} {
		if got := jsonEqual(tc.a, tc.b); got != tc.equal {
			t.Errorf("%s == %s: %v", tc.a, tc.b, got)
		}
	}
}
