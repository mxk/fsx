package index

import (
	"io/fs"
	"slices"
	"strings"
	"time"
)

// File is a regular file in the file system.
type File struct {
	Path
	Digest  Digest
	Size    int64
	ModTime time.Time
	Flag    Flag
}

// IsSame returns whether the file still has the same name, size, and
// modification time.
func (f *File) IsSame(fi fs.FileInfo, err error) bool {
	return err == nil && fi.Mode().IsRegular() && fi.Size() == f.Size &&
		fi.ModTime().Equal(f.ModTime)
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

// Files is an ordered list of files.
type Files []*File

// Sort sorts files by path.
func (fs Files) Sort() {
	slices.SortStableFunc(fs, func(a, b *File) int { return a.Path.cmp(b.Path) })
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
