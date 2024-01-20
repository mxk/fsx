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

// Build builds an index of fsys. If errFn is non-nil, it is called for any
// file-specific errors. If progFn is non-nil, it is called at regular intervals
// to report progress. A non-nil error is returned if ctx is canceled.
func Build(ctx context.Context, fsys fs.FS, errFn func(error), progFn func(*Progress)) (Index, error) {
	file := make(chan *File, 1)
	var werr chan error
	if errFn != nil {
		werr = make(chan error, 1)
	}
	w := walker{fsys: fsys, file: file, werr: werr}
	go w.walk()

	var prog Progress
	var progTick <-chan time.Time
	if progFn != nil {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		progTick = t.C
		prog.reset(time.Now())
	}

	all := make(Files, 0, 4096)
	cancel := ctx.Done()
	for {
		select {
		case f, ok := <-file:
			if !ok {
				if progTick != nil {
					prog.final = true
					prog.update(time.Now())
					progFn(&prog)
				}
				all.Sort()
				return New(dirFSRoot(fsys), all), nil
			}
			if all = append(all, f); progTick != nil {
				prog.fileDone(time.Now(), f.Size)
			}
		case err := <-werr:
			errFn(err)
		case <-progTick:
			progFn(&prog)
		case <-cancel:
			return Index{}, ctx.Err()
		}
	}
}

// walker walks the file system, hashing all regular files.
type walker struct {
	fsys fs.FS
	file chan<- *File
	werr chan<- error
	wg   sync.WaitGroup
}

func (w *walker) walk() {
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
	err := fs.WalkDir(w.fsys, ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			w.err(fmt.Errorf("index: walk error: %s (%w)", name, err))
			err = nil
		} else if strings.IndexByte(name, '\n') >= 0 {
			w.err(fmt.Errorf("index: new line in path: %q", name))
			err = fs.SkipDir
		} else if e.Type().IsRegular() {
			names <- name
		} else if !e.IsDir() {
			w.err(fmt.Errorf("index: not a regular file or directory: %s", name))
		}
		return err
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
		if f, err := h.Read(w.fsys, name); err != nil {
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
