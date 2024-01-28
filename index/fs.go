package index

import (
	"cmp"
	"io/fs"
	"slices"
	"strings"
	"time"
)

// File is a regular file in the file system.
type File struct {
	Path
	digest  Digest
	size    int64
	modTime time.Time
	flag    Flag
}

// Digest returns file digest.
func (f *File) Digest() Digest { return f.digest }

// Size returns file size.
func (f *File) Size() int64 { return f.size }

// ModTime returns file modification time.
func (f *File) ModTime() time.Time { return f.modTime }

// Flag returns file flags.
func (f *File) Flag() Flag { return f.flag }

// isSame returns whether the file still has the same name, size, and
// modification time.
func (f *File) isSame(fi fs.FileInfo, err error) bool {
	return err == nil && fi.Mode().IsRegular() && fi.Size() == f.size &&
		fi.ModTime().Equal(f.modTime)
}

// canIgnore returns whether the specified file name can be ignored for the
// purposes of deduplication.
func (f *File) canIgnore() bool {
	if f.size == 0 {
		return true
	}
	name := f.Base()
	return strings.EqualFold(name, "Thumbs.db") ||
		strings.EqualFold(name, "desktop.ini")
}

// existsIn returns whether f exists in d.
func (f *File) existsIn(d *Dir) bool {
	return !f.flag.IsGone() && d.Path.Contains(f.Path)
}

// isSafeIn returns whether f is a safe file in d.
func (f *File) isSafeIn(d *Dir) bool {
	return f.flag.IsSafe() && d.Path.Contains(f.Path)
}

// isSafeOutsideOf returns whether f is a safe file outside d.
func (f *File) isSafeOutsideOf(d *Dir) bool {
	return f.flag.IsSafe() && !d.Path.Contains(f.Path)
}

// cmp returns -1 if f < other, 0 if f == other, and +1 if f > other.
func (f *File) cmp(other *File) int {
	if c := f.Path.cmp(other.Path); c != 0 {
		return c
	}
	if c := cmp.Compare(f.flag&flagGone, other.flag&flagGone); c != 0 {
		return c
	}
	if c := f.modTime.Compare(other.modTime); c != 0 {
		return c
	}
	if c := cmp.Compare(f.flag&flagKeep, other.flag&flagKeep); c != 0 {
		return c
	}
	return cmp.Compare(f.size, other.size)
}

// Files is an ordered list of files.
type Files []*File

// Sort sorts files by path and other attributes.
func (fs Files) Sort() { slices.SortFunc(fs, (*File).cmp) }

// Dir is a directory in the file system.
type Dir struct {
	Path
	dirs        Dirs  // Subdirectories
	files       Files // Files in this directory
	atom        *Dir  // Atomic container directory, such as .git
	totalDirs   int   // Total number of direct and indirect directories
	totalFiles  int   // Total number of direct and indirect files
	uniqueFiles int   // Total number of direct and indirect unique files
}

// cmp returns -1 if d < other, 0 if d == other, and +1 if d > other.
func (d *Dir) cmp(other *Dir) int { return d.Path.cmp(other.Path) }

// updateCounts updates total directory and file counts. It assumes that no
// files in the tree are marked as gone.
func (d *Dir) updateCounts() {
	d.totalDirs = len(d.dirs)
	d.totalFiles = len(d.files)
	for _, c := range d.dirs {
		c.updateCounts()
		d.totalDirs += c.totalDirs
		d.totalFiles += c.totalFiles
	}
	if d.totalFiles < d.uniqueFiles {
		panic("index: invalid total or unique file count") // Shouldn't happen
	}
}

// Dirs is an ordered list of directories.
type Dirs []*Dir

// Sort sorts files by path and other attributes.
func (ds Dirs) Sort() { slices.SortFunc(ds, (*Dir).cmp) }
