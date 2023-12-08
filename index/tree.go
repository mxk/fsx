package index

import (
	"fmt"
	"math"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Directory names that should be treated as monolithic.
var monolith = map[string]struct{}{
	".git": {},
	".svn": {},
}

// Tree is a directory tree representation of the index.
type Tree struct {
	dirs map[string]*Dir
	idx  map[Digest]Files
}

// ToTree creates a tree representation of the file system index.
func (idx Index) ToTree() *Tree {
	t := &Tree{
		dirs: make(map[string]*Dir, len(idx.Groups)),
		idx:  make(map[Digest]Files, len(idx.Groups)),
	}

	// Add each file to the tree, creating all required Dir entries
	for _, g := range idx.Groups {
		if _, dup := t.idx[g[0].Digest]; dup {
			panic(fmt.Sprintf("digest collision: %x", g[0].Digest))
		}
		t.idx[g[0].Digest] = g
		for _, f := range g {
			t.addFile(f)
		}
	}

	// Replace Dir entries for monolith directories
	var subdirs Dirs
	for p, d := range t.dirs {
		base := path.Base(p)
		if _, ok := monolith[base]; !ok {
			continue
		}
		subdirs = d.appendSubdirs(subdirs[:0])
		for _, sd := range subdirs {
			if _, ok := t.dirs[sd.Path]; !ok {
				panic(fmt.Sprint("missing directory: ", sd.Path))
			}
			d.Files = append(d.Files, sd.Files...)
			t.dirs[sd.Path] = d
		}
	}
	return t
}

// addFile adds file f to the tree, creating all required directory entries.
func (t *Tree) addFile(f *File) {
	p := path.Dir(f.Path)
	if d, ok := t.dirs[p]; ok {
		d.Files = append(d.Files, f)
		return
	}
	dir := &Dir{Path: p, Files: Files{f}}
	t.dirs[p] = dir
	for dir.Path != "." {
		p := path.Dir(dir.Path)
		if d, ok := t.dirs[p]; ok {
			d.Dirs = append(d.Dirs, dir)
			break
		}
		dir = &Dir{Path: p, Dirs: Dirs{dir}}
		t.dirs[p] = dir
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
	root, ok := t.dirs["."]
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
		// on sends. Doing it this way simplifies select block.
		for i := len(queue) - 1; i >= 0; i = len(queue) - 1 {
			select {
			case next <- queue[i][0]:
				if queue[i] = queue[i][1:]; len(queue[i]) == 0 {
					queue = queue[:i]
				}
			default:
				break submit
			}
		}
		select {
		case d, ok := <-dup:
			if !ok {
				sort.Slice(dups, func(i, j int) bool {
					return pathLess(dups[i].Path, dups[j].Path)
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
	dirs := make(map[string]int)
next:
	for root := range next {
		prefix := root.Path + "/"
		subtree = root.appendSubdirs(append(subtree[:0], root))
		clear(safe)
		clear(dirs)
		for _, d := range subtree {
			for _, f := range d.Files {
				if strings.HasSuffix(f.Path, "/Thumbs.db") {
					continue
				}
				for _, dup := range t.idx[f.Digest] {
					if strings.HasPrefix(dup.Path, prefix) {
						continue
					}
					safe[f.Digest] = true
					dirs[path.Dir(dup.Path)]++
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
				bestAlt, maxRefs := "", 0
				for _, dup := range t.idx[f.Digest] {
					if strings.HasPrefix(dup.Path, prefix) {
						continue
					}
					alt := path.Dir(dup.Path)
					if refs := dirs[alt]; maxRefs < refs {
						bestAlt, maxRefs = alt, refs
					}
				}
				if maxRefs < math.MaxInt {
					if used++; bestAlt == "" {
						panic("alt not found")
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
			return pathLess(alt[i].Path, alt[j].Path)
		})
		dup <- &Dup{Dir: root, Alt: alt}
		wg.Done()
	}
}

// Dir is a directory in the file system.
type Dir struct {
	Path  string
	Dirs  Dirs
	Files Files
}

// Dirs is an ordered list of directories.
type Dirs []*Dir

func (d *Dir) appendSubdirs(dirs Dirs) Dirs {
	i := len(dirs)
	for dirs = append(dirs, d.Dirs...); i < len(dirs); i++ {
		dirs = append(dirs, dirs[i].Dirs...)
	}
	return dirs
}
