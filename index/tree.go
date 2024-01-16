package index

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Tree is a directory tree representation of the index.
type Tree struct {
	dirs map[Path]*Dir
	idx  map[Digest]Files
}

// Dir is a directory in the file system.
type Dir struct {
	Path
	Dirs        Dirs
	Files       Files
	Atom        *Dir
	UniqueFiles int
}

// Dirs is an ordered list of directories.
type Dirs []*Dir

// Dup is a directory that can be deleted without losing too much data.
type Dup struct {
	*Dir
	Alt     Dirs  // Directories that contain copies of files in Dir
	Lost    Files // Unique files that would be lost of Dir is deleted
	Ignored Files // Unimportant files that may be lost
}

// Tree creates a tree representation of the index.
func (idx *Index) Tree() *Tree {
	t := &Tree{
		dirs: make(map[Path]*Dir, len(idx.groups)),
		idx:  make(map[Digest]Files, len(idx.groups)),
	}

	// Add each file to the tree, creating all required Dir entries and updating
	// unique file counts.
	var dirs uniqueDirs
	for _, g := range idx.groups {
		if _, dup := t.idx[g[0].Digest]; dup {
			panic(fmt.Sprintf("index: digest collision: %x", g[0].Digest))
		}
		t.idx[g[0].Digest] = g
		for _, f := range g {
			t.addFile(f)
			dirs.add(f.Dir())
		}
		dirs.forEach(func(p Path) { t.dirs[p].UniqueFiles++ })
	}

	// Sort directories and files, and find atomic directories
	subtree := make(dirStack, 0, 16)
	for _, d := range t.dirs {
		sort.Slice(d.Dirs, func(i, j int) bool {
			return d.Dirs[i].Base() < d.Dirs[j].Base()
		})
		sort.Slice(d.Files, func(i, j int) bool {
			return d.Files[i].Base() < d.Files[j].Base()
		})
		if d.Atom == nil && isAtomic(d.Base()) {
			for subtree.from(d); len(subtree) > 0; {
				subtree.next().Atom = d
			}
		}
	}
	return t
}

// addFile adds file f to the tree, creating all required parent directories.
func (t *Tree) addFile(f *File) {
	name := f.Dir()
	if p, ok := t.dirs[name]; ok {
		p.Files = append(p.Files, f)
		return
	}
	d := &Dir{Path: name, Files: Files{f}}
	t.dirs[name] = d
	for name != Root {
		name = d.Dir()
		if p, ok := t.dirs[name]; ok {
			p.Dirs = append(p.Dirs, d)
			break
		}
		d = &Dir{Path: name, Dirs: Dirs{d}}
		t.dirs[name] = d
	}
}

// Dir returns the specified directory if it exists.
func (t *Tree) Dir(name string) *Dir {
	return t.dirs[dirPath(name)]
}

// Dups returns directories under dir that contain duplicate data. If max is >
// 0, at most that many directories are returned.
func (t *Tree) Dups(dir Path, max int) []*Dup {
	root, ok := t.dirs[dir]
	if !ok || len(root.Dirs) == 0 {
		return nil
	}
	queue := []Dirs{root.Dirs}

	// Directories are sent to workers via next. If it's a duplicate, it is
	// returned via dup. Otherwise, its subdirectories are returned via dirs.
	next := make(chan *Dir, runtime.NumCPU())
	dup := make(chan *Dup, 1)
	dirs := make(chan Dirs, 1)
	var wg sync.WaitGroup
	wg.Add(len(queue[0]))
	for i := 0; i < cap(next); i++ {
		go func(next <-chan *Dir, dup chan<- *Dup, dirs chan<- Dirs) {
			defer wg.Done()
			dd := t.newDedup()
			for root := range next {
				if d := dd.dedup(root); d != nil {
					dup <- d
				} else {
					dirs <- root.Dirs
				}
			}
		}(next, dup, dirs)
	}
	go func() {
		wg.Wait() // Wait for all directories to be processed
		wg.Add(cap(next))
		close(next)
		wg.Wait() // Wait for all workers to return
		close(dup)
	}()

	// Process the queue in approximate depth-first order without blocking on
	// sends. This simplifies the select block when the queue is empty, but
	// requires next to have enough capacity for each worker.
	var dups []*Dup
	for {
	send:
		for i := len(queue) - 1; i >= 0; {
			select {
			case next <- queue[i][0]:
				if queue[i] = queue[i][1:]; len(queue[i]) == 0 {
					queue = queue[:i]
					i--
				}
			default:
				break send
			}
		}
		select {
		case d, ok := <-dup:
			if !ok {
				sort.Slice(dups, func(i, j int) bool {
					return dups[i].Path.less(dups[j].Path)
				})
				if max <= 0 || len(dups) < max {
					max = len(dups)
				}
				return dups[:max:len(dups)]
			}
			if dups = append(dups, d); len(dups) == max {
				for _, q := range queue {
					wg.Add(-len(q))
				}
				for queue != nil {
					select {
					case <-next:
						wg.Done()
					default:
						queue = nil
					}
				}
			}
		case d := <-dirs:
			if len(d) > 0 && queue != nil {
				wg.Add(len(d))
				queue = append(queue, d)
			}
		}
		wg.Done()
	}
}

// dedup locates duplicate files in the Tree.
type dedup struct {
	*Tree
	subtree   dirStack
	ignored   Files
	safe      map[Digest]struct{}
	lost      map[Digest]struct{}
	uniqDirs  uniqueDirs
	safeCount map[Path]int
}

func (t *Tree) newDedup() *dedup {
	return &dedup{
		Tree:      t,
		subtree:   make(dirStack, 0, 16),
		safe:      make(map[Digest]struct{}),
		lost:      make(map[Digest]struct{}),
		safeCount: make(map[Path]int),
	}
}

// dedup returns a non-nil Dup if root can be deduplicated.
func (dd *dedup) dedup(root *Dir) *Dup {
	if root.Atom != nil && root.Atom != root {
		return nil
	}

	// Categorize files as ignored, safe, or lost
	dd.ignored = dd.ignored[:0]
	clear(dd.safe)
	clear(dd.lost)
	for dd.subtree.from(root); len(dd.subtree) > 0; {
	files:
		for _, f := range dd.subtree.next().Files {
			if f.canIgnore() {
				dd.ignored = append(dd.ignored, f)
				continue
			}
			// TODO: Check safe and lost first?
			for _, dup := range dd.idx[f.Digest] {
				if !root.Contains(dup.Path) {
					dd.safe[f.Digest] = struct{}{}
					continue files
				}
			}
			if dd.lost[f.Digest] = struct{}{}; len(dd.lost) == 10 {
				return nil
			}
		}
	}
	if len(dd.lost)*len(dd.lost) > len(dd.safe) {
		return nil
	}

	// Set safe file counts for each alternate directory and its parents
	clear(dd.safeCount)
	for g := range dd.safe {
		for _, f := range dd.idx[g] {
			if !root.Contains(f.Path) {
				dd.uniqDirs.add(f.Dir())
			}
		}
		dd.uniqDirs.forEach(func(p Path) { dd.safeCount[p]++ })
	}

	// Record lost and ignored files
	dup := &Dup{Dir: root}
	if len(dd.lost) > 0 {
		dup.Lost = make(Files, 0, len(dd.lost))
		for g := range dd.lost {
			dup.Lost = append(dup.Lost, dd.idx[g]...)
		}
		dup.Lost.Sort()
	}
	if len(dd.ignored) > 0 {
		dup.Ignored = append(make(Files, 0, len(dd.ignored)), dd.ignored...)
		dup.Ignored.Sort()
	}

	// Select alternate directories until all safe files are accounted for
	for len(dd.safe) > 0 {
		// Score alternate directories based on the total number of unique files
		// and the ratio of desired vs. extraneous files. Directories containing
		// root (e.g. ".") are not desirable because they make it hard to
		// determine where the copies are.
		bestScore := -math.MaxFloat64
		var bestDir Path
		for p, n := range dd.safeCount {
			alt := dd.dirs[p]
			if alt.Atom != nil {
				alt = alt.Atom
			}
			score := float64(n) * (float64(n) / float64(alt.UniqueFiles))
			if p.Contains(root.Path) {
				score -= float64(len(dd.safe))
			}
			if bestScore < score {
				bestScore, bestDir = score, alt.Path
			}
		}
		if bestDir.p == "" {
			panic("index: shouldn't happen")
		}

		// Remove bestDir contents from safe and safeCount
		for g := range dd.safe {
			contains := false
			for _, f := range dd.idx[g] {
				if bestDir.Contains(f.Path) {
					contains = true
					break
				}
			}
			if !contains {
				continue
			}
			delete(dd.safe, g)
			for _, f := range dd.idx[g] {
				if !root.Contains(f.Path) {
					dd.uniqDirs.add(f.Dir())
				}
			}
			dd.uniqDirs.forEach(func(p Path) {
				if n := dd.safeCount[p] - 1; n > 0 {
					dd.safeCount[p] = n
				} else {
					delete(dd.safeCount, p)
				}
			})
		}
		dup.Alt = append(dup.Alt, dd.dirs[bestDir])
	}
	return dup
}

// dirStack is a stack of directories that are visited in depth-first order.
type dirStack Dirs

func (s *dirStack) from(root *Dir) {
	*s = append((*s)[:0], root)
}

func (s *dirStack) next() (d *Dir) {
	i := len(*s) - 1
	d, *s = (*s)[i], (*s)[:i]
	for i := len(d.Dirs) - 1; i >= 0; i-- {
		*s = append(*s, d.Dirs[i])
	}
	return
}

// isAtomic returns whether dir is an atomic directory name.
func isAtomic(dir string) bool {
	switch dir {
	case ".git", ".svn":
		return true
	default:
		return false
	}
}

// canIgnore returns whether the specified file name can be ignored for the
// purposes of deduplication.
func (f *File) canIgnore() bool {
	if f.Size == 0 {
		return true
	}
	name := f.Base()
	return strings.EqualFold(name, "Thumbs.db") ||
		strings.EqualFold(name, "desktop.ini")
}
