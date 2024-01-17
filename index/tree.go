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
	Dirs        Dirs  // Subdirectories
	Files       Files // Files in this directory
	Atom        *Dir  // Atomic container directory, such as .git
	UniqueFiles int   // Total number of direct and indirect unique files
}

// Dirs is an ordered list of directories.
type Dirs []*Dir

// Dup is a directory that can be deleted without losing too much data.
type Dup struct {
	*Dir
	Alt     Dirs  // Directories that contain copies of unique files in Dir
	Lost    Files // Unique files that would be lost if Dir is deleted
	Ignored Files // Unimportant files that may be lost if Dir is deleted
}

// Tree creates a tree representation of the index.
func (idx *Index) Tree() *Tree {
	t := &Tree{
		dirs: make(map[Path]*Dir, len(idx.groups)/8),
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
			dirs.add(f.Dir()) // TODO: Don't count files that are ignored?
		}
		dirs.forEach(func(p Path) { t.dirs[p].UniqueFiles++ })
	}

	// Sort directories and files, and find atomic directories
	sortCh := make(chan *Dir, min(runtime.NumCPU(), 8))
	var wg sync.WaitGroup
	wg.Add(cap(sortCh))
	for n := cap(sortCh); n > 0; n-- {
		go func(sortCh <-chan *Dir) {
			defer wg.Done()
			for d := range sortCh {
				sort.Slice(d.Dirs, func(i, j int) bool {
					return d.Dirs[i].Base() < d.Dirs[j].Base()
				})
				sort.Slice(d.Files, func(i, j int) bool {
					return d.Files[i].Base() < d.Files[j].Base()
				})
			}
		}(sortCh)
	}
	var subtree dirStack
	for _, d := range t.dirs {
		sortCh <- d
		if d.Atom == nil && isAtomic(d.Base()) {
			for subtree.from(d); len(subtree) > 0; {
				subtree.next().Atom = d
			}
		}
	}
	close(sortCh)
	wg.Wait()
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

// Dir returns the specified directory, if it exists.
func (t *Tree) Dir(name string) *Dir {
	return t.dirs[dirPath(name)]
}

// Dups returns directories under dir that contain duplicate data. If maxDups is
// > 0, at most that many directories are returned. maxLost is the maximum
// number of files that can be lost for a directory to still be considered a
// duplicate.
func (t *Tree) Dups(dir Path, maxDups, maxLost int) []*Dup {
	root, ok := t.dirs[dir]
	if !ok || len(root.Dirs) == 0 {
		return nil
	}
	queue := dirStack{root.Dirs}

	// Directories are sent to workers via next. Duplicates are returned via
	// dup. Subdirectories of non-duplicates are returned via dirs.
	next := make(chan *Dir, runtime.NumCPU())
	dup := make(chan *Dup, 1)
	dirs := make(chan Dirs, 1)
	var wg sync.WaitGroup
	wg.Add(len(queue[0]))
	for n := cap(next); n > 0; n-- {
		go func(next <-chan *Dir, dup chan<- *Dup, dirs chan<- Dirs) {
			defer wg.Done()
			dd := t.newDedup()
			for root := range next {
				if d := dd.dedup(root, maxLost); d != nil {
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
		for len(queue) > 0 {
			select {
			case next <- queue.peek():
				queue.pop()
			default:
				break send
			}
		}
		select {
		case d, ok := <-dup:
			if !ok {
				// All workers have returned
				sort.Slice(dups, func(i, j int) bool {
					return dups[i].Path.less(dups[j].Path)
				})
				if maxDups <= 0 || len(dups) < maxDups {
					maxDups = len(dups)
				}
				return dups[:maxDups:len(dups)]
			}
			if dups = append(dups, d); len(dups) == maxDups {
				// Cancel all remaining work
				var n int
				for queue != nil {
					select {
					case <-next:
						n++
					default:
						for _, q := range queue {
							n += len(q)
						}
						queue = nil
					}
				}
				wg.Add(-n)
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

// dedup locates duplicate directories in the Tree.
type dedup struct {
	Tree
	subtree    dirStack
	ignored    Files
	safe       map[Digest]struct{}
	lost       map[Digest]struct{}
	uniqueDirs uniqueDirs
	safeCount  map[Path]int
}

func (t *Tree) newDedup() *dedup {
	return &dedup{
		Tree:      *t,
		safe:      make(map[Digest]struct{}),
		lost:      make(map[Digest]struct{}),
		safeCount: make(map[Path]int),
	}
}

// dedup returns a non-nil Dup if root can be deduplicated.
func (dd *dedup) dedup(root *Dir, maxLost int) *Dup {
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
			for _, dup := range dd.idx[f.Digest] {
				if !root.Contains(dup.Path) {
					dd.safe[f.Digest] = struct{}{}
					continue files
				}
			}
			if dd.lost[f.Digest] = struct{}{}; len(dd.lost) > maxLost {
				return nil
			}
		}
	}

	// Require more files to be saved than lost
	if len(dd.safe) <= len(dd.lost)*len(dd.lost) {
		return nil
	}

	// Record ignored and lost files
	dup := &Dup{Dir: root}
	if len(dd.ignored) > 0 {
		dup.Ignored = append(make(Files, 0, len(dd.ignored)), dd.ignored...)
		dup.Ignored.Sort()
	}
	if len(dd.lost) > 0 {
		dup.Lost = make(Files, 0, len(dd.lost))
		for g := range dd.lost {
			dup.Lost = append(dup.Lost, dd.idx[g]...)
		}
		dup.Lost.Sort()
	}

	// Create per-directory safe file counts
	clear(dd.safeCount)
	for g := range dd.safe {
		for _, f := range dd.idx[g] {
			if !root.Contains(f.Path) {
				dd.uniqueDirs.add(f.Dir())
			}
		}
		dd.uniqueDirs.forEach(func(p Path) { dd.safeCount[p]++ })
	}

	// Select alternate directories until all safe files are accounted for. At
	// each iteration, we select an alternate directory based on a quality
	// score, remove all files it contains from dd.safe, decrement dd.safeCount,
	// and repeat the process with the next best directory.
	//
	// Scoring is based on the total number of unique files and the ratio of
	// desired vs. extraneous files. Directories containing root are not
	// desirable because they make it hard to determine where the copies are.
	// Directories closer to root are slightly preferred for easier navigation.
	// Ties are broken by path sort order to ensure deterministic output.
	for len(dd.safe) > 0 {
		bestScore, bestDir := -math.MaxFloat64, Path{}
		for p, n := range dd.safeCount {
			alt := dd.dirs[p]
			if alt.Atom != nil {
				alt = alt.Atom // TODO: Should n be changed?
			}
			score := float64(n)*(float64(1+n)/float64(1+alt.UniqueFiles)) +
				2.0/float64(1+root.Dist(p))
			if p.Contains(root.Path) {
				score -= float64(len(dd.safe))
			}
			if bestScore < score || (bestScore == score && alt.Path.less(bestDir)) {
				bestScore, bestDir = score, alt.Path
			}
		}
		if bestDir.p == "" {
			panic("index: failed to find alternate directory") // Shouldn't happen
		}

		// Remove bestDir contents from safe and safeCount
		for g := range dd.safe {
			// TODO: Iterate over bestDir files instead?
			group := dd.idx[g]
			for _, f := range group {
				if bestDir.Contains(f.Path) {
					goto match
				}
			}
			continue
		match:
			delete(dd.safe, g)
			for _, f := range group {
				if !root.Contains(f.Path) {
					dd.uniqueDirs.add(f.Dir())
				}
			}
			dd.uniqueDirs.forEach(func(p Path) {
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
// Each entry must be non-empty to ensure that len(stack) > 0 implies a
// non-empty stack.
type dirStack []Dirs

// from initializes the stack with the specified directory.
func (s *dirStack) from(root *Dir) {
	*s = append((*s)[:0], Dirs{root})
}

// peek returns the next directory without removing it from the stack.
func (s *dirStack) peek() (d *Dir) {
	if i := len(*s) - 1; i >= 0 {
		d = (*s)[i][0]
	}
	return
}

// pop returns the next directory from the stack.
func (s *dirStack) pop() (d *Dir) {
	if i := len(*s) - 1; i >= 0 {
		d, (*s)[i] = (*s)[i][0], (*s)[i][1:]
		if len((*s)[i]) == 0 {
			*s = (*s)[:i]
		}
	}
	return
}

// next returns the next directory and adds its children to the stack.
func (s *dirStack) next() (d *Dir) {
	if d = s.pop(); d != nil && len(d.Dirs) > 0 {
		*s = append(*s, d.Dirs)
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
