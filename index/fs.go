package index

import (
	"cmp"
	"fmt"
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

// isSafeOutsideOf returns whether f is a safe file outside of d.
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

// altScore returns a quality score in the range [0,1] representing the
// similarity of alt to d, with alt containing a subset of safe unique files out
// of rem remaining unique files that are yet to be saved in d.
//
// Scoring is based on multiple factors, including the total number of unique
// files out of those remaining and the ratio of desired vs. extraneous files
// (the root directory contains all files in d, but also many others, making it
// the worst possible choice). Directories containing d are not desirable
// because they make it hard to verify preservation. Directories closer to d are
// preferred for easier navigation. The number of subdirectories is minimized to
// ensure the most specific match.
//
// If d contains only unique files, an exact copy of it located in the same
// parent directory receives a score of 1.
func (d *Dir) altScore(alt *Dir, safe, rem int) float64 {
	if !(0 < safe && safe <= alt.uniqueFiles) ||
		!(safe <= rem && rem <= d.uniqueFiles) {
		panic("index: invalid file counts") // Shouldn't happen
	}

	// A perfect match is safe == rem == alt.uniqueFiles, meaning that alt is an
	// exact subset of d in terms of unique files.
	s := float64(safe)
	match := (s / float64(rem)) * (s / float64(alt.uniqueFiles))

	// A perfect match minimizes the total file count, meaning that alt has
	// exactly one copy of each unique file and nothing else.
	files := s / float64(alt.totalFiles)

	// A perfect match minimizes the total directory count, meaning that alt has
	// a flat structure. The main reason for this is that if alt can be any
	// directory in `X/Y/Z/` with only Z containing any files (same total and
	// unique file counts for all three directories), we want to pick Z.
	dirs := 1 / (1 + float64(alt.totalDirs))

	// A perfect match is close to d. We only count the number of steps to the
	// common root because we don't want to penalize more specific matches.
	dist := 1 / float64(d.Dist(d.CommonRoot(alt.Path)))

	// TODO: Favor directories with flagKeep files

	// Total score favors a perfect match over everything else.
	const a = 1.0 / 8
	score := (5*a)*match + a*files + a*dirs + a*dist

	// A perfect match does not contain d.
	if alt.Contains(d.Path) {
		score /= 2
	}
	if !(0 <= score && score <= 1) {
		panic(fmt.Sprintf("index: invalid score: %f", score))
	}
	return score
}

// Dirs is an ordered list of directories.
type Dirs []*Dir

// Sort sorts files by path and other attributes.
func (ds Dirs) Sort() {
	slices.SortFunc(ds, func(a, b *Dir) int { return a.Path.cmp(b.Path) })
}
