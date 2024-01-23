package index

import "fmt"

// Flag specifies file flags.
type Flag byte

const (
	flagNone Flag = iota      // Zero value
	flagDup                   // File may be removed
	flagJunk                  // File and all of its copies may be removed
	flagKeep                  // File must be preserved
	flagGone Flag = 1 << iota // File no longer exists
	flagSame                  // File exists and hasn't changed (runtime only)

	flagRuntime = flagSame // Runtime only flags
)

const (
	flagDupS  = 'D'
	flagJunkS = 'J'
	flagKeepS = 'K'
	flagGoneS = 'X'
)

// IsDup returns whether this file is a duplicate that may be removed.
func (a Flag) IsDup() bool { return a&flagKeep == flagDup }

// IsJunk returns whether the file and all of its copies may be removed.
func (a Flag) IsJunk() bool { return a&flagKeep == flagJunk }

// Keep returns whether the file must be preserved.
func (a Flag) Keep() bool { return a&flagKeep == flagKeep }

// IsGone returns whether the file no longer exists.
func (a Flag) IsGone() bool { return a&flagGone != 0 }

// MayRemove returns whether the file may be removed.
func (a Flag) MayRemove() bool { return a&flagKeep == flagDup || a&flagKeep == flagJunk }

// persist returns whether the file should be written to the index.
func (a Flag) persist() bool { return a&flagGone == 0 || a&flagKeep != 0 }

// String returns the string representation of file flags.
func (a Flag) String() string {
	switch a & (flagKeep | flagGone) {
	case flagNone:
		return ""
	case flagDup:
		return "D"
	case flagJunk:
		return "J"
	case flagKeep:
		return "K"
	case flagDup | flagGone:
		return "DX"
	case flagJunk | flagGone:
		return "JX"
	case flagKeep | flagGone:
		return "KX"
	default:
		panic(fmt.Sprintf("index: invalid flag value: 0x%X", byte(a)))
	}
}

// parseFlag decodes the string representation of file flags.
func parseFlag[T string | []byte](b T) (a Flag, ok bool) {
	if len(b) == 0 {
		return flagNone, true
	}
	ok = len(b) == 1
	if b[len(b)-1] == flagGoneS {
		a = flagGone
		ok = len(b) == 2
	}
	switch b[0] {
	case flagDupS:
		a |= flagDup
	case flagJunkS:
		a |= flagJunk
	case flagKeepS:
		a |= flagKeep
	default:
		ok = false
	}
	return
}
