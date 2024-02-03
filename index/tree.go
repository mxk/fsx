package index

import (
	"cmp"
	"fmt"
	"io/fs"
	"runtime"
	"slices"
	"sync"
)

// Tree is a directory tree representation of the index.
type Tree struct {
	root string
	dirs map[path]*dir
	idx  map[Digest]Files
}

// ToTree converts from an index to a tree representation.
func (x *Index) ToTree() *Tree {
	if len(x.groups) == 0 {
		return &Tree{root: x.root, dirs: map[path]*dir{".": {path: "."}}}
	}
	t := &Tree{
		root: x.root,
		dirs: make(map[path]*dir, len(x.groups)/8),
		idx:  make(map[Digest]Files, len(x.groups)),
	}
	t.dirs["."] = &dir{path: "."}

	// Add each file to the tree, creating all required dir entries and updating
	// unique file counts.
	var dirs uniqueDirs
	for _, g := range x.groups {
		if _, dup := t.idx[g[0].digest]; dup {
			panic(fmt.Sprintf("index: digest collision: %x", g[0].digest))
		}
		t.idx[g[0].digest] = g
		for _, f := range g {
			if !f.flag.IsGone() {
				t.addFile(f)
				dirs.add(f.dir()) // TODO: Don't count files that are ignored?
			}
		}
		dirs.forEach(func(p path) { t.dirs[p].uniqueFiles++ })
	}

	// Sort directories and files, and find atomic directories
	sort := make(chan *dir, min(runtime.NumCPU(), 8))
	var wg sync.WaitGroup
	wg.Add(cap(sort))
	for n := cap(sort); n > 0; n-- {
		go func(sort <-chan *dir) {
			defer wg.Done()
			for d := range sort {
				slices.SortFunc(d.dirs, func(a, b *dir) int {
					if c := cmp.Compare(a.base(), b.base()); c != 0 {
						return c
					}
					panic(fmt.Sprint("index: duplicate directory name: ", a))
				})
				slices.SortFunc(d.files, func(a, b *File) int {
					if c := cmp.Compare(a.base(), b.base()); c != 0 {
						return c
					}
					panic(fmt.Sprint("index: duplicate file name: ", a))
				})
			}
		}(sort)
	}
	var subtree dirStack
	for _, d := range t.dirs {
		sort <- d
		if d.atom == nil && isAtomic(d.base()) {
			for subtree.from(d); len(subtree) > 0; {
				subtree.next().atom = d
			}
		}
	}
	close(sort)
	wg.Wait()

	// Update directory and file counts
	t.dirs["."].updateCounts()
	if _, ok := t.dirs[""]; ok { // Sanity check
		panic("index: corrupt directory tree")
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

// File returns the specified file or nil if it does not exist.
func (t *Tree) File(name string) *File { return t.file(filePath(name)) }

// MarkDup sets the duplicate flag for a single file or all files under a
// directory. Files that are already marked are unaffected.
func (t *Tree) MarkDup(name string) error { return t.mark(name, flagDup) }

// MarkJunk sets the junk flag for a single file or all files under a directory.
// Files that are already marked are unaffected.
func (t *Tree) MarkJunk(name string) error { return t.mark(name, flagJunk) }

// MarkKeep sets the keep flag for a single file or all files under a directory.
// Files that are already marked are unaffected.
func (t *Tree) MarkKeep(name string) error { return t.mark(name, flagKeep) }

// mark sets the specified flag for a single file or all files under a
// directory.
func (t *Tree) mark(name string, flag Flag) error {
	set := func(f *File, flag Flag) {
		if f.flag&flagKeep == 0 {
			f.flag |= flag
		}
	}
	if flag == 0 || flag&^flagKeep != 0 {
		panic("index: invalid flag")
	}
	dir, file := eitherPath(name)
	if d := t.dirs[dir]; d != nil {
		var dirs dirStack
		for dirs.from(d); len(dirs) > 0; {
			for _, f := range dirs.next().files {
				set(f, flag)
			}
		}
		return nil
	}
	if f := t.file(file); f != nil {
		set(f, flag)
		return nil
	}
	return fmt.Errorf("index: %w: %s", fs.ErrNotExist, name)
}

// Dups returns directories under dir that contain duplicate data. If maxDups is
// > 0, at most that many directories are returned. maxLost is the maximum
// number of unique files that can be lost for a directory to still be
// considered a duplicate.
func (t *Tree) Dups(dirName string, maxDups, maxLost int) []*Dup {
	root := t.dir(dirName)
	if root == nil || len(root.dirs) == 0 {
		return nil
	}
	var q dirStack
	q.from(root.dirs...)

	// Directories are sent to workers via next. Duplicates are returned via
	// dup. Subdirectories of non-duplicates are returned via todo.
	next := make(chan *dir, runtime.NumCPU())
	dup := make(chan *Dup, 1)
	todo := make(chan dirs, 1)
	var wg sync.WaitGroup
	wg.Add(len(q))
	for n := cap(next); n > 0; n-- {
		go func(next <-chan *dir, dup chan<- *Dup, todo chan<- dirs) {
			defer wg.Done()
			var dd dedup
			for root := range next {
				if dd.isDup(t, root.path, maxLost) {
					dup <- dd.dedup()
				} else {
					todo <- root.dirs
				}
			}
		}(next, dup, todo)
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
				slices.SortFunc(dups, func(a, b *Dup) int { return a.path.cmp(b.path) })
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
		case d := <-todo:
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
	name := f.dir()
	if d := t.dirs[name]; d != nil {
		d.files = append(d.files, f)
		return
	}
	d := &dir{path: name, files: Files{f}}
	t.dirs[name] = d
	for name != "." {
		name = d.dir()
		if p := t.dirs[name]; p != nil {
			p.dirs = append(p.dirs, d)
			break
		}
		d = &dir{path: name, dirs: dirs{d}}
		t.dirs[name] = d
	}
}

// dir returns the specified directory or nil if it does not exist.
func (t *Tree) dir(name string) *dir { return t.dirs[dirPath(name)] }

// file returns the specified file, if it exists.
func (t *Tree) file(p path) *File {
	if p == "" {
		return nil
	}
	d := t.dirs[p.dir()]
	if d == nil || p.isDir() {
		return nil
	}
	base := p.base()
	i, ok := slices.BinarySearchFunc(d.files, base, func(f *File, base string) int {
		return cmp.Compare(f.base(), base)
	})
	if ok {
		return d.files[i]
	}
	return nil
}

// dirStack is a stack of directories that are visited in depth-first order.
type dirStack dirs

// from initializes the stack with the specified directories.
func (s *dirStack) from(ds ...*dir) {
	if *s = (*s)[:0]; cap(*s) < len(ds) {
		*s = make(dirStack, 0, max(2*len(ds), 16))
	}
	s.push(ds)
}

// push pushes ds in reverse order to the stack.
func (s *dirStack) push(ds dirs) {
	if len(ds) <= 1 {
		*s = append(*s, ds...)
		return
	}
	*s = append(*s, make(dirs, len(ds))...)
	for i, j := len(*s)-len(ds), len(ds)-1; j >= 0; i, j = i+1, j-1 {
		(*s)[i] = ds[j]
	}
}

// next returns the next directory and adds its children to the stack.
func (s *dirStack) next() (d *dir) {
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
