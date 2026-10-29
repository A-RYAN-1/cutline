package cutline

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"
)

// fate is what happens to a pull request that landed on stage.
type fate int

const (
	fateStay       fate = iota // never promoted
	fatePickX                  // every commit promoted with "cherry-pick -x"
	fatePickPlain              // every commit promoted without the trailer
	fatePartial                // merge style only: first commit promoted, rest not
	fateResolved               // re-created on prod as one squash commit with different content, same subject
	fatePickEdited             // merge only: every commit re-done by hand with the trailer, so the patch differs
)

// model is the reference: what the history was built from, and so what the
// report has to say. It does not look at git.
type model struct {
	stage  map[int]fate // pull request number -> fate
	hotfix map[int]bool // pull requests that landed on prod only
	style  map[int]bool // true: merge commit, false: squash
	size   map[int]int  // commits in the pull request
}

// build lands random pull requests on stage, promotes some of them to prod in
// a shuffled order and lands some hotfixes on prod directly.
func build(r *repo, rng *rand.Rand) *model {
	m := &model{stage: map[int]fate{}, hotfix: map[int]bool{}, style: map[int]bool{}, size: map[int]int{}}
	type landed struct {
		pr   int
		shas []string // squash: one sha; merge: the member commits
	}
	var lands []landed
	next := 1
	r.on("stage")
	for i, n := 0, 3+rng.Intn(6); i < n; i++ {
		pr := next
		next++
		merge := rng.Intn(2) == 0
		k := 1
		if merge {
			k = 1 + rng.Intn(3)
		}
		var msgs []string
		for j := 0; j < k; j++ {
			msgs = append(msgs, fmt.Sprintf("work %d.%d", pr, j))
		}
		var shas []string
		if merge {
			shas = r.mergePR(strconv.Itoa(pr), msgs...)
		} else {
			shas = []string{r.squashPR(strconv.Itoa(pr), fmt.Sprintf("Feature %d", pr), msgs...)}
		}
		f := fate(rng.Intn(6))
		if f == fatePartial && (!merge || k < 2) {
			f = fatePickX
		}
		if f == fatePickEdited && !merge {
			f = fatePickX
		}
		m.stage[pr], m.style[pr], m.size[pr] = f, merge, len(shas)
		lands = append(lands, landed{pr, shas})
	}
	rng.Shuffle(len(lands), func(i, j int) { lands[i], lands[j] = lands[j], lands[i] })
	r.on("prod")
	for _, l := range lands {
		switch m.stage[l.pr] {
		case fatePickX:
			for _, s := range l.shas {
				r.git("cherry-pick", "-x", s)
			}
		case fatePickPlain:
			for _, s := range l.shas {
				r.git("cherry-pick", s)
			}
		case fatePartial:
			r.git("cherry-pick", "-x", l.shas[0])
		case fatePickEdited:
			for _, sha := range l.shas {
				r.n++
				r.write("edited"+strconv.Itoa(r.n)+".txt", "edited by hand\n")
				r.git("add", "-A")
				r.git("commit", "-q", "-m", "edited pick", "-m", "(cherry picked from commit "+sha+")")
			}
		case fateResolved:
			r.n++
			r.write("resolved"+strconv.Itoa(l.pr)+".txt", "resolved by hand\n")
			r.git("add", "-A")
			r.git("commit", "-q", "-m", fmt.Sprintf("Feature %d (#%d)", l.pr, l.pr))
		}
	}
	for i, n := 0, rng.Intn(3); i < n; i++ {
		pr := next
		next++
		r.squashPR(strconv.Itoa(pr), fmt.Sprintf("Hotfix %d", pr), fmt.Sprintf("fix %d", pr))
		m.hotfix[pr] = true
	}
	return m
}

func prs(es []Entry) []int {
	var out []int
	for _, e := range es {
		out = append(out, e.PR)
	}
	sort.Ints(out)
	return out
}

func (m *model) want() (onlyA, onlyB, both []int) {
	for pr, f := range m.stage {
		switch f {
		case fateStay, fatePartial:
			onlyA = append(onlyA, pr)
		default:
			both = append(both, pr)
		}
	}
	for pr := range m.hotfix {
		onlyB = append(onlyB, pr)
	}
	sort.Ints(onlyA)
	sort.Ints(onlyB)
	sort.Ints(both)
	return
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func seedCount() int {
	if s := os.Getenv("CUTLINE_SEEDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	if testing.Short() {
		return 5
	}
	return 40
}

// TestMatcherAgainstModel checks the report against the reference model on
// random histories mixing squash and merge-commit pull requests, picks with and
// without the trailer, partial picks and hotfixes on prod.
func TestMatcherAgainstModel(t *testing.T) {
	for seed := 1; seed <= seedCount(); seed++ {
		seed := seed
		t.Run("seed"+strconv.Itoa(seed), func(t *testing.T) {
			t.Parallel()
			r := newRepo(t)
			m := build(r, rand.New(rand.NewSource(int64(seed))))
			res := r.diff(Options{})
			wa, wb, wboth := m.want()
			if got := prs(res.OnlyA); !equal(got, wa) {
				t.Errorf("OnlyA = %v, want %v", got, wa)
			}
			if got := prs(res.OnlyB); !equal(got, wb) {
				t.Errorf("OnlyB = %v, want %v", got, wb)
			}
			if got := prs(res.Both); !equal(got, wboth) {
				t.Errorf("Both = %v, want %v", got, wboth)
			}
			for _, e := range res.OnlyA {
				want := StatusMissing
				if m.stage[e.PR] == fatePartial {
					want = StatusPartial
				}
				if e.Status != want {
					t.Errorf("PR %d status = %s, want %s", e.PR, e.Status, want)
				}
				if len(e.Commits) != m.size[e.PR] {
					t.Errorf("PR %d lists %d commits, want %d", e.PR, len(e.Commits), m.size[e.PR])
				}
			}
			// The same histories read the other way round swap the sides.
			back, err := Diff(context.Background(), "prod", "stage", Options{Dir: r.dir})
			if err != nil {
				t.Fatal(err)
			}
			if got := prs(back.OnlyB); !equal(got, wa) {
				t.Errorf("reversed OnlyB = %v, want %v", got, wa)
			}
			if got := prs(back.OnlyA); !equal(got, wb) {
				t.Errorf("reversed OnlyA = %v, want %v", got, wb)
			}
		})
	}
}
