package cutline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// VerifyOptions for Verify.
type VerifyOptions struct {
	Options
	// Expect are pull requests that must be in B, for example the ones a plan
	// promoted. Empty means everything in A must be in B.
	Expect []int
}

// Verification is the outcome of checking that B has what A has.
type Verification struct {
	From string `json:"from"`
	Into string `json:"into"`
	// Missing are expected changes that are absent from B or only partly there.
	Missing []Entry `json:"missing"`
	// Ahead are changes in B that A does not have, such as a hotfix that was
	// never merged back.
	Ahead []Entry `json:"ahead"`
	// OK is true when Missing and Ahead are both empty.
	OK bool `json:"ok"`
	// Notices are non-fatal problems, such as a refused hosting-service request.
	Notices []string `json:"notices,omitempty"`
}

// Verify checks that b has everything expected from a, and reports changes in b
// that a lacks.
func Verify(ctx context.Context, a, b string, opt VerifyOptions) (*Verification, error) {
	res, err := Diff(ctx, a, b, opt.Options)
	if err != nil {
		return nil, err
	}
	want := map[int]bool{}
	for _, n := range opt.Expect {
		want[n] = true
	}
	v := &Verification{From: a, Into: b, Missing: []Entry{}, Ahead: res.OnlyB, Notices: res.Notices}
	seen := map[int]bool{}
	for _, e := range res.OnlyA {
		if len(want) == 0 || (e.PR != 0 && want[e.PR]) {
			v.Missing = append(v.Missing, e)
		}
	}
	for _, e := range res.Both {
		seen[e.PR] = true
	}
	for _, e := range res.OnlyA {
		seen[e.PR] = true
	}
	for n := range want {
		if !seen[n] {
			return nil, fmt.Errorf("pull request #%d is not in %s", n, a)
		}
	}
	v.OK = len(v.Missing) == 0 && len(v.Ahead) == 0
	return v, nil
}

// Write renders the verification. Text and markdown are the same.
func (v *Verification) Write(w io.Writer, f Format) error {
	switch f {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case FormatText, FormatMarkdown, "":
		p := &printer{w: w}
		if v.OK {
			p.printf("OK: %s has everything %s has, and nothing %s lacks.\n", v.Into, v.From, v.From)
			writeNotices(p, v.Notices)
			return p.err
		}
		p.printf("NOT OK\n")
		if len(v.Missing) > 0 {
			p.printf("Not in %s yet (%d):\n", v.Into, len(v.Missing))
			for _, e := range v.Missing {
				p.printf("  %s\n", e.line())
			}
		}
		if len(v.Ahead) > 0 {
			p.printf("In %s but not in %s (%d), merge back:\n", v.Into, v.From, len(v.Ahead))
			for _, e := range v.Ahead {
				p.printf("  %s\n", e.line())
			}
		}
		writeNotices(p, v.Notices)
		return p.err
	}
	return fmt.Errorf("unknown format %q (use text, markdown or json)", f)
}
