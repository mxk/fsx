package index

import (
	"fmt"
	"math"
)

// Dup is a directory that can be deleted without losing too much data.
type Dup struct {
	*Dir
	Alt     Dirs  // Directories that contain copies of unique files in Dir
	Lost    Files // Unique files that would be lost if Dir is deleted
	Ignored Files // Unimportant files that may be lost if Dir is deleted
}

// dedup maintains directory deduplication state to minimize allocations.
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

// isDup returns whether root can be deduplicated. This is a relatively fast
// operation that simply ensures that every unique file under root, except those
// that can be ignored, has at least one copy outside of root that is not marked
// for possible removal. maxLost is the maximum number of unique files that can
// be lost for a directory to still be considered a duplicate.
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
	return dd.root != nil
}

// dedup returns the deduplication strategy for the directory passed to isDup.
// It may only be called once after a call to isDup returned true.
func (dd *dedup) dedup() *Dup {
	if dd.root == nil {
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
				if f.existsIn(dd.root) {
					dup.Lost = append(dup.Lost, f)
				}
			}
		}
		dup.Lost.Sort()
	}

	// Select alternate directories until all safe files are accounted for
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
			if len(dd.uniqueDirs) == 0 {
				panic("index: no alternates for a safe file") // Shouldn't happen
			}
			dd.uniqueDirs.forEach(func(p Path) { dd.safeCount[p]++ })
		}

		// Find the next best alternate
		maxScore, bestAlt := math.Inf(-1), (*Dir)(nil)
		for p, n := range dd.safeCount {
			d := dd.tree.dirs[p]
			s := dd.root.altScore(d, n, len(dd.safe))
			if maxScore < s || (maxScore == s && d.cmp(bestAlt) < 0) {
				maxScore, bestAlt = s, d
			}
		}

		// Remove all safe files under bestAlt from dd.safe
		for g := range dd.safe {
			for _, f := range dd.tree.idx[g] {
				if f.isSafeIn(bestAlt) {
					delete(dd.safe, g)
					break
				}
			}
		}
		dup.Alt = append(dup.Alt, bestAlt)
	}
	dup.Alt.Sort()
	dd.tree, dd.root = nil, nil
	return dup
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
