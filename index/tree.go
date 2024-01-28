package index

import (
	"cmp"
	"fmt"
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
	if len(idx.groups) == 0 {
		return &Tree{root: idx.root}
	}
	t := &Tree{
		root: idx.root,
		dirs: make(map[Path]*Dir, len(idx.groups)/8),
		idx:  make(map[Digest]Files, len(idx.groups)),
	}

	// Add each file to the tree, creating all required Dir entries and updating
	// unique file counts.
	var dirs uniqueDirs
	for _, g := range idx.groups {
		if _, dup := t.idx[g[0].digest]; dup {
			panic(fmt.Sprintf("index: digest collision: %x", g[0].digest))
		}
		t.idx[g[0].digest] = g
		for _, f := range g {
			if !f.flag.IsGone() {
				t.addFile(f)
				dirs.add(f.Dir()) // TODO: Don't count files that are ignored?
			}
		}
		dirs.forEach(func(p Path) { t.dirs[p].uniqueFiles++ })
	}

	// Sort directories and files, and find atomic directories
	sort := make(chan *Dir, min(runtime.NumCPU(), 8))
	var wg sync.WaitGroup
	wg.Add(cap(sort))
	for n := cap(sort); n > 0; n-- {
		go func(sort <-chan *Dir) {
			defer wg.Done()
			for d := range sort {
				slices.SortFunc(d.dirs, func(a, b *Dir) int {
					if c := cmp.Compare(a.Base(), b.Base()); c != 0 {
						return c
					}
					panic(fmt.Sprintf("index: duplicate directory name: %s", a))
				})
				slices.SortFunc(d.files, func(a, b *File) int {
					if c := cmp.Compare(a.Base(), b.Base()); c != 0 {
						return c
					}
					panic(fmt.Sprintf("index: duplicate file name: %s", a))
				})
			}
		}(sort)
	}
	var subtree dirStack
	for _, d := range t.dirs {
		sort <- d
		if d.atom == nil && isAtomic(d.Base()) {
			for subtree.from(d); len(subtree) > 0; {
				subtree.next().atom = d
			}
		}
	}
	close(sort)
	wg.Wait()

	// Update directory and file counts
	if root := t.dirs[Root]; root != nil {
		root.updateCounts()
	} else {
		t.dirs = nil
	}
	return t
}

// ToIndex converts from a tree to index representation.
func (t *Tree) ToIndex() *Index {
	all := make(Files, 0, len(t.idx))
	for _, g := range t.idx {
		all = append(all, g...)
	}
	all.Sort()
	return New(t.root, all)
}

// Dir returns the specified directory, if it exists.
func (t *Tree) Dir(name string) *Dir {
	return t.dirs[dirPath(name)]
}

// Dups returns directories under dir that contain duplicate data. If maxDups is
// > 0, at most that many directories are returned. maxLost is the maximum
// number of unique files that can be lost for a directory to still be
// considered a duplicate.
func (t *Tree) Dups(dir Path, maxDups, maxLost int) []*Dup {
	root := t.dirs[dir]
	if root == nil || len(root.dirs) == 0 {
		return nil
	}
	var q dirStack
	q.from(root.dirs...)

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
				if dd.isDup(t, root, maxLost) {
					dup <- dd.dedup()
				} else {
					dirs <- root.dirs
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

// addFile adds file f to the tree, creating any required parent directories.
func (t *Tree) addFile(f *File) {
	name := f.Dir()
	if d := t.dirs[name]; d != nil {
		d.files = append(d.files, f)
		return
	}
	d := &Dir{Path: name, files: Files{f}}
	t.dirs[name] = d
	for name != Root {
		name = d.Dir()
		if p := t.dirs[name]; p != nil {
			p.dirs = append(p.dirs, d)
			break
		}
		d = &Dir{Path: name, dirs: Dirs{d}}
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
	i, ok := slices.BinarySearchFunc(d.files, base, func(f *File, base string) int {
		return cmp.Compare(f.Base(), base)
	})
	if ok {
		return d.files[i]
	}
	return nil
}

// dedup locates duplicate directories in the Tree.
type dedup struct {
	tree *Tree
	root *Dir

	subtree dirStack
	ignored Files
	safe    map[Digest]struct{}
	lost    map[Digest]struct{}

	uniqueDirs uniqueDirs
	safeCount  map[Path]int
}

// isDup returns whether root can be deduplicated.
func (dd *dedup) isDup(tree *Tree, root *Dir, maxLost int) bool {
	dd.tree, dd.root = nil, nil
	if root.atom != nil && root.atom != root {
		return false
	}
	if dd.safe == nil {
		dd.safe = make(map[Digest]struct{})
		dd.lost = make(map[Digest]struct{})
	} else {
		clear(dd.safe)
		clear(dd.lost)
	}

	// Categorize files as ignored, safe, or lost
	dd.ignored = dd.ignored[:0]
	for dd.subtree.from(root); len(dd.subtree) > 0; {
	files:
		for _, f := range dd.subtree.next().files {
			if f.flag&flagPersist != 0 {
				// Tree shouldn't contain files marked gone, but just in case
				if f.flag.IsGone() {
					continue
				}
				if f.flag.Keep() {
					return false
				}
			}
			if f.canIgnore() {
				dd.ignored = append(dd.ignored, f)
				continue
			}
			if g := tree.idx[f.digest]; len(g) > 1 {
				for _, dup := range g {
					if dup.isSafeOutsideOf(root) {
						dd.safe[f.digest] = struct{}{}
						continue files
					}
				}
			}
			if dd.lost[f.digest] = struct{}{}; len(dd.lost) > maxLost {
				return false
			}
		}
	}

	// Require more unique files to be saved than lost
	if len(dd.safe) > len(dd.lost)*len(dd.lost) {
		dd.tree, dd.root = tree, root
	}
	return dd.tree != nil
}

// dedup returns a non-nil Dup if isDup returned true.
func (dd *dedup) dedup() *Dup {
	if dd.tree == nil {
		return nil
	}

	// Record ignored and lost files
	dup := &Dup{Dir: dd.root}
	if len(dd.ignored) > 0 {
		dup.Ignored = append(make(Files, 0, len(dd.ignored)), dd.ignored...)
		dup.Ignored.Sort()
	}
	if len(dd.lost) > 0 {
		dup.Lost = make(Files, 0, len(dd.lost))
		for g := range dd.lost {
			for _, f := range dd.tree.idx[g] {
				if !f.flag.IsGone() && dd.root.Contains(f.Path) {
					dup.Lost = append(dup.Lost, f)
				}
			}
		}
		dup.Lost.Sort()
	}

	// Select alternate directories until all safe files are accounted for. At
	// each iteration we count the total number of safe files in each directory
	// and its subdirectories, pick the directory with the highest score, remove
	// all files it contains from dd.safe, and repeat the process.
	dd.uniqueDirs = dd.uniqueDirs[:0]
	if dd.safeCount == nil {
		dd.safeCount = make(map[Path]int)
	}
	for len(dd.safe) > 0 {
		// Create per-directory safe file counts
		clear(dd.safeCount)
		for g := range dd.safe {
			for _, f := range dd.tree.idx[g] {
				if f.isSafeOutsideOf(dd.root) {
					d := dd.tree.dirs[f.Dir()]
					if d.atom != nil {
						d = d.atom
					}
					dd.uniqueDirs.add(d.Path)
				}
			}
			dd.uniqueDirs.forEach(func(p Path) { dd.safeCount[p]++ })
		}

		// Find the highest score
		maxScore, maxDir := 0.0, Path{}
		for p, n := range dd.safeCount {
			score := dd.root.altScore(dd.tree.dirs[p], n, len(dd.safe))
			if maxScore < score || (maxScore == score && p.cmp(maxDir) < 0) {
				maxScore, maxDir = score, p
			}
		}
		if maxDir == (Path{}) {
			panic("index: failed to find alternate directory") // Shouldn't happen
		}

		// Remove maxDir contents from safe
		for g := range dd.safe {
			for _, f := range dd.tree.idx[g] {
				if maxDir.Contains(f.Path) {
					delete(dd.safe, g)
					break
				}
			}
		}
		dup.Alt = append(dup.Alt, dd.tree.dirs[maxDir])
	}
	dup.Alt.Sort()
	dd.tree, dd.root = nil, nil
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
		s.push(d.dirs)
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
