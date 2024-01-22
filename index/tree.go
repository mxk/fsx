package index

import (
	"cmp"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"
)

// Tree is a directory tree representation of the index.
type Tree struct {
	root string
	dirs map[Path]*Dir
	idx  map[Digest]Files
}

// Dup is a directory that can be deleted without losing too much data.
type Dup struct {
	*Dir
	Alt     Dirs  // Directories that contain copies of unique files in Dir
	Lost    Files // Unique files that would be lost if Dir is deleted
	Ignored Files // Unimportant files that may be lost if Dir is deleted
}

// ToTree converts from an index to a tree representation.
func (idx *Index) ToTree() *Tree {
	t := &Tree{
		root: idx.root,
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
			if !f.Flag.IsGone() {
				t.addFile(f)
				dirs.add(f.Dir()) // TODO: Don't count files that are ignored?
			}
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
				slices.SortFunc(d.Dirs, func(a, b *Dir) int {
					return cmp.Compare(a.Base(), b.Base())
				})
				slices.SortFunc(d.Files, func(a, b *File) int {
					return cmp.Compare(a.Base(), b.Base())
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

// ToIndex converts from a tree to index representation.
func (t *Tree) ToIndex() Index {
	groups := make([]Files, 0, len(t.idx))
	for _, g := range t.idx {
		groups = append(groups, g)
	}
	// TODO: Maintain original group order if there are identical paths with
	// different digests.
	slices.SortFunc(groups, func(a, b Files) int { return a[0].Path.cmp(b[0].Path) })
	return Index{t.root, groups}
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
	var q dirStack
	q.from(root.Dirs...)

	// Directories are sent to workers via next. Duplicates are returned via
	// dup. Subdirectories of non-duplicates are returned via dirs.
	next := make(chan *Dir, runtime.NumCPU())
	dup := make(chan *Dup, 1)
	dirs := make(chan Dirs, 1)
	var wg sync.WaitGroup
	wg.Add(len(q))
	for n := cap(next); n > 0; n-- {
		go func(next <-chan *Dir, dup chan<- *Dup, dirs chan<- Dirs) {
			defer wg.Done()
			var dd dedup
			for root := range next {
				if d := dd.dedup(t, root, maxLost); d != nil {
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
		for len(q) > 0 {
			select {
			case next <- q[len(q)-1]:
				q = q[:len(q)-1]
			default:
				break send
			}
		}
		select {
		case d, ok := <-dup:
			if !ok {
				// All workers have returned
				slices.SortFunc(dups, func(a, b *Dup) int { return a.Path.cmp(b.Path) })
				if maxDups <= 0 || len(dups) < maxDups {
					maxDups = len(dups)
				}
				return dups[:maxDups:len(dups)]
			}
			if dups = append(dups, d); len(dups) == maxDups {
				// Cancel all remaining work
				n := len(q)
				for q != nil {
					select {
					case <-next:
						n++
					default:
						q = nil
					}
				}
				wg.Add(-n)
			}
		case d := <-dirs:
			if len(d) > 0 && q != nil {
				wg.Add(len(d))
				q.push(d)
			}
		}
		wg.Done()
	}
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

// file returns the specified file, if it exists.
func (t *Tree) file(p Path) *File {
	d := t.dirs[p.Dir()]
	if p.IsDir() || d == nil {
		return nil
	}
	base := p.Base()
	i, ok := slices.BinarySearchFunc(d.Files, base, func(f *File, base string) int {
		return cmp.Compare(f.Base(), base)
	})
	if ok {
		return d.Files[i]
	}
	return nil
}

// dedup locates duplicate directories in the Tree.
type dedup struct {
	subtree    dirStack
	ignored    Files
	safe       map[Digest]struct{}
	lost       map[Digest]struct{}
	safeCount  map[Path]int
	uniqueDirs uniqueDirs
}

// dedup returns a non-nil Dup if root can be deduplicated.
func (dd *dedup) dedup(tree *Tree, root *Dir, maxLost int) *Dup {
	if root.Atom != nil && root.Atom != root {
		return nil
	}
	if dd.safe == nil {
		dd.safe = make(map[Digest]struct{})
		dd.lost = make(map[Digest]struct{})
		dd.safeCount = make(map[Path]int)
	} else {
		clear(dd.safe)
		clear(dd.lost)
		clear(dd.safeCount)
	}

	// Categorize files as ignored, safe, or lost
	dd.ignored = dd.ignored[:0]
	for dd.subtree.from(root); len(dd.subtree) > 0; {
	files:
		for _, f := range dd.subtree.next().Files {
			if f.canIgnore() {
				dd.ignored = append(dd.ignored, f)
				continue
			}
			for _, dup := range tree.idx[f.Digest] {
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
			dup.Lost = append(dup.Lost, tree.idx[g]...)
		}
		dup.Lost.Sort()
	}

	// Create per-directory safe file counts
	for g := range dd.safe {
		for _, f := range tree.idx[g] {
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
			alt := tree.dirs[p]
			if alt.Atom != nil {
				alt = alt.Atom // TODO: Should n be changed?
			}
			score := float64(n)*(float64(1+n)/float64(1+alt.UniqueFiles)) +
				2.0/float64(1+root.Dist(p))
			if p.Contains(root.Path) {
				score -= float64(len(dd.safe))
			}
			if bestScore < score || (bestScore == score && alt.Path.cmp(bestDir) < 0) {
				bestScore, bestDir = score, alt.Path
			}
		}
		if bestDir.p == "" {
			panic("index: failed to find alternate directory") // Shouldn't happen
		}

		// Remove bestDir contents from safe and safeCount
		for g := range dd.safe {
			// TODO: Iterate over bestDir files instead?
			group := tree.idx[g]
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
		dup.Alt = append(dup.Alt, tree.dirs[bestDir])
	}
	return dup
}

// dirStack is a stack of directories that are visited in depth-first order.
type dirStack Dirs

// from initializes the stack with the specified directories.
func (s *dirStack) from(ds ...*Dir) {
	if *s = (*s)[:0]; cap(*s) < len(ds) {
		*s = make(dirStack, 0, max(2*len(ds), 16))
	}
	s.push(ds)
}

// push pushes ds in reverse order to the stack.
func (s *dirStack) push(ds Dirs) {
	if len(ds) <= 1 {
		*s = append(*s, ds...)
		return
	}
	*s = append(*s, make(Dirs, len(ds))...)
	for i, j := len(*s)-len(ds), len(ds)-1; j >= 0; i, j = i+1, j-1 {
		(*s)[i] = ds[j]
	}
}

// next returns the next directory and adds its children to the stack.
func (s *dirStack) next() (d *Dir) {
	if i := len(*s) - 1; i >= 0 {
		d, *s = (*s)[i], (*s)[:i]
		s.push(d.Dirs)
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
