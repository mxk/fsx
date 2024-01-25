package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/mxk/go-cli"

	"github.com/mxk/fsx/index"
)

var _ = cli.Main.Add(&cli.Cfg{
	Name:    "hash",
	Usage:   "file ...",
	Summary: "Calculate BLAKE3 digests for one or more files",
	MinArgs: 1,
	New:     func() cli.Cmd { return &hashCmd{} },
})

type hashCmd struct {
	Cmp bool `cli:"Report whether file contents are identical"`
}

func (cmd *hashCmd) Main(args []string) error {
	if len(args) == 1 {
		r := hash(index.NewHasher(nil), args, 0)
		if r.print(); r.err != nil {
			r.err = cli.ExitCode(1)
		}
		return r.err
	}

	// Hash files
	var next, workers atomic.Int32
	done := make(chan *hashResult, 1)
	for n := workers.Add(int32(min(len(args), runtime.NumCPU()))); n > 0; n-- {
		go func() {
			h := index.NewHasher(nil)
			for i := next.Load(); int(i) < len(args); i = next.Load() {
				if next.CompareAndSwap(i, i+1) {
					done <- hash(h, args, int(i))
				}
			}
			if workers.Add(-1) == 0 {
				close(done)
			}
		}()
	}

	// Receive results
	var q hashQueue
	for r := range done {
		q.result(r)
	}
	if cmd.Cmp && q.err == nil {
		what := "identical"
		if q.diff {
			what = "different"
			q.err = cli.ExitCode(1)
		}
		_, _ = fmt.Fprintln(os.Stderr, "Files are", what)
	}
	return q.err
}

type hashQueue struct {
	next int
	done map[int]*hashResult
	want index.Digest
	diff bool
	err  error
}

func (q *hashQueue) result(r *hashResult) {
	if r.err != nil {
		q.err = cli.ExitCode(1)
	}
	if !q.diff {
		if q.want == (index.Digest{}) {
			q.want = r.Digest()
		} else if q.want != r.Digest() {
			q.diff = true
		}
	}
	if r.i != q.next {
		if q.done == nil {
			q.done = make(map[int]*hashResult)
		}
		q.done[r.i] = r
		return
	}
	for {
		r.print()
		q.next++
		if r = q.done[q.next]; r == nil {
			break
		}
		delete(q.done, q.next)
	}
}

type hashResult struct {
	*index.File
	i    int
	name string
	err  error
}

func hash(h *index.Hasher, names []string, i int) *hashResult {
	r := &hashResult{i: i, name: filepath.Clean(names[i])}
	r.File, r.err = h.Read(nil, r.name, false)
	return r
}

func (r *hashResult) print() {
	if r.err != nil {
		_, _ = fmt.Fprintln(os.Stderr, r.err)
	} else {
		fmt.Printf("%X  %s\n", r.Digest(), r.name)
	}
}
