package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/mxk/go-cli"

	"github.com/mxk/fsx/index"
)

var _ = cli.Main.Add(&cli.Cfg{
	Name:    "index",
	Usage:   "<out-db> <dir>",
	Summary: "Create an index file",
	MinArgs: 2,
	MaxArgs: 2,
	New:     func() cli.Cmd { return &indexCmd{} },
})

type indexCmd struct{}

func (cmd *indexCmd) Main(args []string) error {
	files := make(chan *index.File, 1)
	go walkFS(os.DirFS(args[1]), files)
	all := make(index.Files, 0, 4096)
	stats := NewStats()
	for file := range files {
		all = append(all, file)
		stats.addFile(file.Size)
	}
	stats.report()
	all.Sort()
	idx := index.Index{Root: args[1], Groups: groupByDigest(all)}
	return cli.WriteFileAtomic(args[0], func(f *os.File) error {
		return idx.Write(f)
	})
}

func walkFS(fsys fs.FS, files chan<- *index.File) {
	paths := make(chan string, 1)
	wg := new(sync.WaitGroup)
	defer func() {
		close(paths)
		wg.Wait()
		close(files)
	}()
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hashFiles(fsys, paths, files)
		}()
	}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if strings.IndexByte(path, '\n') >= 0 {
			panic(fmt.Sprintf("New line in path: %q", path))
		}
		if err != nil {
			log.Printf("Walk error: %s (%v)", path, err)
		} else if d.Type().IsRegular() {
			paths <- path
		} else if !d.IsDir() {
			log.Printf("Not a directory or file: %s", path)
		}
		return nil
	})
	if err != nil {
		log.Println("Final walk error:", err)
	}
}

// hashFiles computes the digest of each file received from paths and sends the
// corresponding File to files.
func hashFiles(fsys fs.FS, paths <-chan string, files chan<- *index.File) {
	h := index.NewHasher()
	for path := range paths {
		if f, err := h.Read(fsys, path); err != nil {
			log.Print(err)
		} else {
			files <- f
		}
	}
}

func groupByDigest(all index.Files) []index.Files {
	type group struct {
		order int
		files index.Files
	}
	idx := make(map[index.Digest]group, len(all))
	for i, f := range all {
		g, ok := idx[f.Digest]
		if !ok {
			g.order = i
		} else if g.files[0].Size != f.Size {
			panic(fmt.Sprintf("Hash collision: %q != %q", g.files[0].Path, f.Path))
		}
		g.files = append(g.files, f)
		idx[f.Digest] = g
	}
	groups := make([]index.Files, 0, len(idx))
	for _, g := range idx {
		groups = append(groups, g.files)
	}
	sort.Slice(groups, func(i, j int) bool {
		return idx[groups[i][0].Digest].order < idx[groups[j][0].Digest].order
	})
	return groups
}

type Stats struct {
	start      time.Time
	lastReport time.Time
	files      uint64
	bytes      uint64
}

func NewStats() Stats {
	t := time.Now()
	return Stats{
		start:      t,
		lastReport: t,
	}
}

func (s *Stats) addFile(bytes int64) {
	s.files++
	s.bytes += uint64(bytes)
	if time.Since(s.lastReport) >= 5*time.Minute {
		s.report()
	}
}

func (s *Stats) report() {
	s.lastReport = time.Now()
	dur := s.lastReport.Sub(s.start)
	sec := dur.Seconds()
	fps := float64(s.files) / sec
	bps := uint64(math.Round(float64(s.bytes) / sec))
	log.Printf("Processed %d files (%s) in %v [%.0f files/sec, %s/sec]",
		s.files, humanize.IBytes(s.bytes), dur, fps, humanize.IBytes(bps))
}
