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
	Dirs  Dirs
	Files Files
}

// Dirs is an ordered list of directories.
type Dirs []*Dir

// Tree creates a tree representation of the index.
func (idx *Index) Tree() *Tree {
	t := &Tree{
		dirs: make(map[Path]*Dir, len(idx.groups)),
		idx:  make(map[Digest]Files, len(idx.groups)),
	}

	// Add each file to the tree, creating all required Dir entries
	for _, g := range idx.groups {
		if _, dup := t.idx[g[0].Digest]; dup {
			panic(fmt.Sprintf("index: digest collision: %x", g[0].Digest))
		}
		t.idx[g[0].Digest] = g
		for _, f := range g {
			t.addFile(f)
		}
	}

	// Replace child Dir entries for monolith directories
	subdirs := make(Dirs, 0, 8)
dirs:
	for name, d := range t.dirs {
		if d.Path != name || !isMonolith(name.Base()) {
			continue
		}
		// Ensure that this is the topmost monolith
		for p := name.Dir(); p != Root; p = p.Dir() {
			if isMonolith(p.Base()) {
				continue dirs
			}
		}
		subdirs = d.appendSubdirs(subdirs[:0])
		d.Dirs = nil
		for _, sd := range subdirs {
			if _, ok := t.dirs[sd.Path]; !ok {
				panic(fmt.Sprint("index: missing directory: ", sd.Path))
			}
			d.Files = append(d.Files, sd.Files...)
			sd.Dirs, sd.Files = nil, nil
			t.dirs[sd.Path] = d
		}
	}

	// Sort directories and files
	for _, d := range t.dirs {
		sort.Slice(d.Dirs, func(i, j int) bool {
			return d.Dirs[i].Base() < d.Dirs[j].Base()
		})
		d.Files.Sort()
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

// Dup is a directory that can be deleted without losing any data.
type Dup struct {
	*Dir
	Alt   Dirs
	Extra Files
}

// Dups returns all directories that contain duplicate data.
func (t *Tree) Dups() []*Dup {
	root, ok := t.dirs[Root]
	if !ok || len(root.Dirs) == 0 {
		return nil
	}

	// Directories are sent via next. If the directory can be safely removed, it
	// is returned via dup. Otherwise, its subdirectories are returned via todo.
	next := make(chan *Dir, runtime.NumCPU())
	todo := make(chan Dirs, 1)
	dup := make(chan *Dup, 1)
	wg := new(sync.WaitGroup)
	wg.Add(len(root.Dirs))
	for i := 0; i < cap(next); i++ {
		go t.findDups(next, todo, dup, wg)
	}
	go func() {
		wg.Wait() // Wait for all directories to be processed
		wg.Add(cap(next))
		close(next)
		wg.Wait() // Wait for all findDups to return
		close(dup)
	}()

	var dups []*Dup
	queue := []Dirs{root.Dirs}
	for {
	submit:
		// Process the queue in approximate depth-first order without blocking
		// on sends. Doing it this way simplifies the select block when the
		// queue is empty.
		for i := len(queue) - 1; i >= 0; {
			select {
			case next <- queue[i][0]:
				if queue[i] = queue[i][1:]; len(queue[i]) == 0 {
					queue = queue[:i]
					i--
				}
			default:
				break submit
			}
		}
		select {
		case d, ok := <-dup:
			if !ok {
				sort.Slice(dups, func(i, j int) bool {
					return dups[i].less(dups[j].Path)
				})
				return dups
			}
			dups = append(dups, d)
		case d := <-todo:
			if len(d) > 0 {
				queue = append(queue, d)
			}
		}
	}
}

// findDups attempts to find alternate locations for files contained in each
// directory received from next.
func (t *Tree) findDups(next <-chan *Dir, todo chan<- Dirs, dup chan<- *Dup, wg *sync.WaitGroup) {
	defer wg.Done()
	var subtree Dirs
	safe := make(map[Digest]bool)
	dirs := make(map[Path]int)
next:
	for root := range next {
		subtree = root.appendSubdirs(append(subtree[:0], root))
		clear(safe)
		clear(dirs)
		for _, d := range subtree {
			for _, f := range d.Files {
				if canIgnore(f.Base()) {
					continue
				}
				for _, dup := range t.idx[f.Digest] {
					if root.Contains(dup.Path) {
						continue
					}
					safe[f.Digest] = true
					dirs[dup.Dir()]++
				}
				if !safe[f.Digest] {
					todo <- root.Dirs
					wg.Add(len(root.Dirs))
					wg.Done()
					continue next
				}
			}
		}
		used := 0
		for _, d := range subtree {
			for _, f := range d.Files {
				if !safe[f.Digest] {
					continue
				}
				bestAlt, maxRefs := Root, 0
				for _, dup := range t.idx[f.Digest] {
					if root.Contains(dup.Path) {
						continue
					}
					alt := dup.Path.Dir()
					if refs := dirs[alt]; maxRefs < refs {
						bestAlt, maxRefs = alt, refs
					}
				}
				if maxRefs < math.MaxInt {
					if used++; bestAlt == Root {
						panic("index: alt not found")
					}
					dirs[bestAlt] = math.MaxInt
				}
			}
		}
		alt := make(Dirs, 0, used)
		for p, refs := range dirs {
			if refs == math.MaxInt {
				alt = append(alt, t.dirs[p])
			}
		}
		sort.Slice(alt, func(i, j int) bool {
			return alt[i].less(alt[j].Path)
		})
		dup <- &Dup{Dir: root, Alt: alt}
		wg.Done()
	}
}

// appendSubdirs appends all direct and indirect subdirectories of d to dirs in
// breadth-first order.
func (d *Dir) appendSubdirs(dirs Dirs) Dirs {
	i := len(dirs)
	for dirs = append(dirs, d.Dirs...); i < len(dirs); i++ {
		dirs = append(dirs, dirs[i].Dirs...)
	}
	return dirs
}

// isMonolith returns whether dir is a monolithic directory name.
func isMonolith(dir string) bool {
	switch dir {
	case ".git", ".svn":
		return true
	default:
		return false
	}
}

// canIgnore returns whether the specified file name can be ignored.
func canIgnore(name string) bool {
	return strings.EqualFold(name, "Thumbs.db") ||
		strings.EqualFold(name, "desktop.ini")
}
