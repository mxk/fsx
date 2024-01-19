package index

import (
	"fmt"
	"unsafe"
)

// Flag specifies file flags.
type Flag byte

const (
	flagNone Flag = iota   // Zero value
	flagDup                // File may be removed
	flagJunk               // File and all of its copies may be removed
	flagKeep               // File must be preserved
	flagGone Flag = 1 << 7 // File no longer exists
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

// String returns the string representation of file flags.
func (a Flag) String() string {
	if a == 0 {
		return ""
	}
	if a&^(flagKeep|flagGone) != 0 {
		panic(fmt.Sprintf("index: invalid flag value: 0x%X", byte(a)))
	}
	b := make([]byte, 0, 2)
	switch a & flagKeep {
	case flagDup:
		b = append(b, flagDupS)
	case flagJunk:
		b = append(b, flagJunkS)
	case flagKeep:
		b = append(b, flagKeepS)
	}
	if a.IsGone() {
		b = append(b, flagGoneS)
	}
	return unsafe.String(&b[0], len(b))
}

// parseFlag decodes the string representation of file flags. It panics if the
// string is invalid.
func parseFlag(b []byte) (a Flag) {
	if len(b) == 0 {
		return
	}
	ok := len(b) == 1
	if b[len(b)-1] == flagGoneS {
		if a = flagGone; ok {
			return
		}
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
	if ok {
		return
	}
	panic(fmt.Sprintf("index: invalid file flag: %s", b))
}
