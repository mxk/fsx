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
// identical names, sizes, and modification times. See Scan for more info.
func (t *Tree) Rescan(ctx context.Context, fsys fs.FS, errFn func(error), progFn func(*Progress)) (Index, error) {
	// Clear runtime flags
	if t != nil {
		for _, g := range t.idx {
			for _, f := range g {
				f.flag &^= flagRuntime
			}
		}
	}

	// Setup workers and progress
	file := make(chan *File, 1)
	var werr chan error
	if errFn != nil {
		werr = make(chan error, 1)
	}
	w := walker{ref: t, fsys: fsys, file: file, werr: werr}
	go w.walk(ctx)

	var prog Progress
	var progTick <-chan time.Time
	if progFn != nil {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		progTick = t.C
		prog.reset(time.Now())
	}

	// Receive files from walk and hash goroutines
	all := make(Files, 0, 64)
recv:
	for {
		select {
		case f, ok := <-file:
			if !ok {
				break recv
			}
			if all = append(all, f); progTick != nil {
				prog.fileDone(time.Now(), f.size)
			}
		case err := <-werr:
			errFn(err)
		case <-progTick:
			progFn(&prog)
		}
	}
	if progTick != nil {
		prog.final = true
		prog.update(time.Now())
		progFn(&prog)
	}
	select {
	case <-ctx.Done():
		return Index{}, ctx.Err()
	default:
	}

	// Copy over removed and modified files that have important flags
	if t != nil {
		for _, g := range t.idx {
			for _, f := range g {
				if f.flag&flagSame == 0 {
					f.flag |= flagGone
					if f.flag&^(flagGone|flagRuntime) != 0 {
						all = append(all, f)
					}
				}
			}
		}
	}
	all.Sort()
	return New(dirFSRoot(fsys), all), nil
}

// walker walks the file system, hashing all regular files.
type walker struct {
	ref  *Tree
	fsys fs.FS
	file chan<- *File
	werr chan<- error
	wg   sync.WaitGroup
}

func (w *walker) walk(ctx context.Context) {
	names := make(chan string, 1)
	defer func() {
		close(names)
		w.wg.Wait()
		close(w.file)
	}()
	for n := runtime.NumCPU(); n > 0; n-- {
		w.wg.Add(1)
		go w.hash(names)
	}
	cancel := ctx.Done()
	err := fs.WalkDir(w.fsys, ".", func(name string, e fs.DirEntry, err error) error {
		if cancel != nil {
			select {
			case <-cancel:
				return fs.SkipAll
			default:
			}
		}
		if err != nil {
			w.err(fmt.Errorf("index: walk error: %s (%w)", name, err))
			return nil
		}
		if strings.IndexByte(name, '\n') >= 0 {
			w.err(fmt.Errorf("index: new line in path: %q", name))
			return fs.SkipDir
		}
		if e.Type().IsRegular() {
			if w.ref != nil {
				// TODO: Is changing case a problem?
				if f := w.ref.file(Path{name}); f != nil && f.IsSame(e.Info()) {
					f.flag |= flagSame
					w.file <- f
					return nil
				}
			}
			names <- name
		} else if !e.IsDir() {
			w.err(fmt.Errorf("index: not a regular file or directory: %s", name))
		}
		return nil
	})
	if err != nil {
		w.err(fmt.Errorf("index: walk error: %w", err))
	}
}

func (w *walker) err(err error) {
	if w.werr != nil {
		w.werr <- err
	}
}

func (w *walker) hash(names <-chan string) {
	defer w.wg.Done()
	h := NewHasher()
	for name := range names {
		// TODO: Cancellation
		if f, err := h.Read(w.fsys, name, true); err != nil {
			w.werr <- err
		} else {
			w.file <- f
		}
	}
}

// Progress reports file indexing progress.
type Progress struct {
	start    time.Time
	now      time.Time
	files    uint64
	bytes    uint64
	newFiles uint64
	newBytes uint64
	fps      float64
	bps      float64
	final    bool
}

// reset resets progress to the specified start time.
func (p *Progress) reset(start time.Time) {
	*p = Progress{start: start, now: start}
}

// Time returns the time when the progress was last updated.
func (p *Progress) Time() time.Time { return p.now }

// IsFinal returns whether this is the final progress report.
func (p *Progress) IsFinal() bool { return p.final }

func (p *Progress) String() string {
	bytes := humanize.IBytes(p.bytes)
	dur := p.now.Sub(p.start).Round(time.Second)
	bps := humanize.IBytes(uint64(math.Round(p.bps)))
	return fmt.Sprintf("Indexed %d files (%s) in %v [%.0f files/sec, %s/sec]",
		p.files, bytes, dur, p.fps, bps)
}

func (p *Progress) fileDone(now time.Time, bytes int64) {
	p.newFiles++
	p.newBytes += uint64(bytes)
	if now.Sub(p.now) >= time.Second {
		p.update(now)
	}
}

func (p *Progress) update(now time.Time) {
	sec := now.Sub(p.now).Seconds()
	if sec <= 0 {
		return
	}
	alpha := min(sec/60, 1)
	if p.start.Equal(p.now) {
		alpha = 1 // First sample
	}
	p.now = now
	p.files += p.newFiles
	p.bytes += p.newBytes
	p.fps = (1-alpha)*p.fps + alpha*(float64(p.newFiles)/sec)
	p.bps = (1-alpha)*p.bps + alpha*(float64(p.newBytes)/sec)
	p.newFiles = 0
	p.newBytes = 0
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
