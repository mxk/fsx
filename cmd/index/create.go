package index

import (
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/mxk/go-cli"

	"github.com/mxk/fsx/index"
)

var _ = indexCli.Add(&cli.Cfg{
	Name:    "create|c",
	Usage:   "<out-db> <dir>",
	Summary: "Create a new file system index",
	MinArgs: 2,
	MaxArgs: 2,
	New:     func() cli.Cmd { return indexCreateCmd{} },
})

type indexCreateCmd struct{}

func (indexCreateCmd) Main(args []string) error {
	root := filepath.Clean(args[1])
	files := make(chan *index.File, 1)
	w := walker{fsys: os.DirFS(root), files: files}
	go w.walk()

	all := make(index.Files, 0, 4096)
	stats := newStats()
	for f := range files {
		all = append(all, f)
		stats.addFile(f.Size)
	}
	stats.report()

	all.Sort()
	idx := index.New(root, all)
	err := cli.WriteFileAtomic(args[0], func(f *os.File) error {
		return idx.Write(f)
	})
	if err == nil && w.err.Load() {
		err = cli.ExitCode(1)
	}
	return err
}

// walker walks the file system, hashing all regular files.
type walker struct {
	fsys  fs.FS
	files chan<- *index.File
	wg    sync.WaitGroup
	err   atomic.Bool
}

func (w *walker) walk() {
	names := make(chan string, 1)
	defer func() {
		close(names)
		w.wg.Wait()
		close(w.files)
	}()
	for n := runtime.NumCPU(); n > 0; n-- {
		w.wg.Add(1)
		go w.hash(names)
	}
	err := fs.WalkDir(w.fsys, ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			w.errorf("Walk error: %s (%v)", name, err)
			err = nil
		} else if strings.IndexByte(name, '\n') >= 0 {
			w.errorf("New line in path: %q", name)
			err = fs.SkipDir
		} else if e.Type().IsRegular() {
			names <- name
		} else if !e.IsDir() {
			w.errorln("Not a directory or file:", name)
		}
		return err
	})
	if err != nil {
		w.errorln("Walk error:", err)
	}
}

func (w *walker) hash(names <-chan string) {
	defer w.wg.Done()
	h := index.NewHasher()
	for name := range names {
		if f, err := h.Read(w.fsys, name); err != nil {
			w.error(err)
		} else {
			w.files <- f
		}
	}
}

func (w *walker) error(v ...any) {
	w.err.Store(true)
	log.Print(v...)
}

func (w *walker) errorf(format string, v ...any) {
	w.err.Store(true)
	log.Printf(format, v...)
}

func (w *walker) errorln(v ...any) {
	w.err.Store(true)
	log.Println(v...)
}

// stats reports file hashing progress.
type stats struct {
	start      time.Time
	lastReport time.Time
	files      uint64
	bytes      uint64
}

func newStats() stats {
	t := time.Now()
	return stats{start: t, lastReport: t}
}

func (s *stats) addFile(bytes int64) {
	s.files++
	s.bytes += uint64(bytes)
	if time.Since(s.lastReport) >= 5*time.Minute {
		s.report()
	}
}

func (s *stats) report() {
	s.lastReport = time.Now()
	dur := s.lastReport.Sub(s.start)
	sec := dur.Seconds()
	fps := float64(s.files) / sec
	bps := uint64(math.Round(float64(s.bytes) / sec))
	log.Printf("Processed %d files (%s) in %v [%.0f files/sec, %s/sec]",
		s.files, humanize.IBytes(s.bytes), dur, fps, humanize.IBytes(bps))
}
