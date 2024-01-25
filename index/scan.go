package index

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
)

// Scan creates an index of fsys. If errFn is non-nil, it is called for any
// file-specific errors. If progFn is non-nil, it is called at regular intervals
// to report progress. A non-nil error is returned if ctx is canceled.
func Scan(ctx context.Context, fsys fs.FS, errFn func(error), progFn func(*Progress)) (Index, error) {
	return (*Tree)(nil).Rescan(ctx, fsys, errFn, progFn)
}

// Rescan updates the index of fsys, skipping the hashing of any files that have
// identical names, sizes, and modification times. See Scan for more info. Tree
// t should not be accessed after this operation.
func (t *Tree) Rescan(ctx context.Context, fsys fs.FS, errFn func(error), progFn func(*Progress)) (Index, error) {
	// Clear non-persistent flags
	if t != nil {
		for _, g := range t.idx {
			for _, f := range g {
				f.flag &= flagPersist
			}
		}
	}

	// Setup progress tracking
	var prog *Progress
	var progTick <-chan time.Time
	if progFn != nil {
		prog = newProgress(time.Now())
		t := time.NewTicker(time.Second)
		defer t.Stop()
		progTick = t.C
	}

	// Start walker and hasher goroutines
	file := make(chan *File, 1)
	var werr chan error
	if errFn != nil {
		werr = make(chan error, 1)
	}
	cp := ctxPoller(ctx.Done())
	go (&walker{fsys: fsys, file: file, werr: werr}).walk(cp, t, func(n int) error {
		if prog != nil {
			prog.sampleBytes.Add(uint64(n))
		}
		if !cp.canceled() {
			return nil
		}
		return context.Canceled
	})

	// Receive files from walk and hash goroutines
	all := make(Files, 0, 64)
recv:
	for {
		select {
		case f, ok := <-file:
			if !ok {
				break recv
			}
			if all = append(all, f); prog != nil {
				prog.sampleFiles++
			}
		case err := <-werr:
			errFn(err)
		case now := <-progTick:
			prog.update(now)
			progFn(prog)
		}
	}
	if prog != nil {
		prog.final = true
		prog.update(time.Now())
		progFn(prog)
	}
	if cp.canceled() {
		return Index{}, ctx.Err()
	}

	// all describes current contents of fsys. Files marked flagSame are shared
	// with t. All other files in t have been either removed or modified, so we
	// mark them with flagGone. Those that have any flagKeep flags are copied
	// over to preserve prior decisions.
	if t != nil {
		for _, g := range t.idx {
			for _, f := range g {
				if f.flag&flagSame != 0 {
					continue // Already in all
				}
				if f.flag |= flagGone; f.flag&flagKeep != 0 {
					all = append(all, f)
				}
			}
		}
	}
	all.Sort()
	return New(dirFSRoot(fsys), all), nil
}

// walker walks the file system, hashing all regular files.
type walker struct {
	fsys fs.FS
	file chan<- *File
	werr chan<- error
	wg   sync.WaitGroup
}

func (w *walker) walk(cp ctxPoller, t *Tree, mon func(int) error) {
	hash := make(chan string, 1)
	defer func() {
		close(hash)
		w.wg.Wait()
		close(w.file)
	}()
	for n := runtime.NumCPU(); n > 0; n-- {
		w.wg.Add(1)
		go w.hash(hash, mon)
	}
	err := fs.WalkDir(w.fsys, ".", func(name string, e fs.DirEntry, err error) error {
		if cp.canceled() {
			return fs.SkipAll
		}
		if err != nil {
			w.err(fmt.Errorf("index: walk error: %s (%w)", name, err))
			return nil
		}
		if len(name) == 0 || name[0] == '\t' || strings.IndexByte(name, '\n') >= 0 {
			w.err(fmt.Errorf("index: unsupported file path: %q", name))
			return fs.SkipDir
		}
		if e.Type().IsRegular() {
			if t != nil {
				if f := t.file(Path{name}); f != nil && f.isSame(e.Info()) {
					f.flag = f.flag&^flagGone | flagSame
					w.file <- f
					return nil
				}
			}
			hash <- name
		} else if !e.IsDir() {
			w.err(fmt.Errorf("index: not a regular file or directory: %s", name))
		}
		return nil
	})
	if err != nil {
		w.err(fmt.Errorf("index: walk error: %w", err))
	}
}

func (w *walker) hash(names <-chan string, mon func(int) error) {
	defer w.wg.Done()
	h := NewHasher(mon)
	for name := range names {
		if f, err := h.Read(w.fsys, name, true); err == nil {
			w.file <- f
		} else if err != context.Canceled {
			w.err(err)
		}
	}
}

func (w *walker) err(err error) {
	if w.werr != nil {
		w.werr <- err
	}
}

// ctxPoller simplifies polling context.Context for cancellation.
type ctxPoller <-chan struct{}

func (p ctxPoller) canceled() bool {
	if p != nil {
		select {
		case <-p:
			return true
		default:
		}
	}
	return false
}

// Progress reports file indexing progress.
type Progress struct {
	sampleFiles uint64
	sampleBytes atomic.Uint64

	start time.Time
	now   time.Time
	dur   time.Duration

	files uint64
	bytes uint64
	fps   float64
	bps   float64
	final bool
}

// newProgress creates a new Progress with the specified start time.
func newProgress(start time.Time) *Progress {
	return &Progress{start: start, now: start}
}

// Duration returns the duration of the operation rounded to the nearest second.
func (p *Progress) Duration() time.Duration { return p.dur }

// IsFinal returns whether this is the final progress report.
func (p *Progress) IsFinal() bool { return p.final }

func (p *Progress) String() string {
	files := humanize.Comma(int64(p.files))
	bytes := humanize.IBytes(p.bytes)
	bps := humanize.IBytes(uint64(math.Round(p.bps)))
	return fmt.Sprintf("Indexed %s files (%s) in %v [%.0f files/sec, %s/sec]",
		files, bytes, p.dur, p.fps, bps)
}

func (p *Progress) update(now time.Time) {
	sampleBytes := p.sampleBytes.Swap(0)
	sec := now.Sub(p.now).Seconds()
	if sec < 0.5 {
		p.sampleBytes.Add(sampleBytes)
		return
	}
	alpha := min(sec/10, 1)
	if p.start.Equal(p.now) {
		alpha = 1 // First sample
	}
	p.now = now
	p.dur = now.Sub(p.start).Round(time.Second)
	p.files += p.sampleFiles
	p.bytes += sampleBytes
	p.fps = (1-alpha)*p.fps + alpha*(float64(p.sampleFiles)/sec)
	p.bps = (1-alpha)*p.bps + alpha*(float64(sampleBytes)/sec)
	p.sampleFiles = 0
}

// dirFSRoot returns the root directory name if fsys refers to the local file
// system (e.g. os.DirFS).
func dirFSRoot(fsys fs.FS) string {
	f, err := fsys.Open(".")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	if f, ok := f.(*os.File); ok {
		return filepath.Clean(f.Name())
	}
	return ""
}
