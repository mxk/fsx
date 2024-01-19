package index

import (
	"fmt"
	"unsafe"
)

// Attr specifies file attributes.
type Attr byte

const (
	attrNone Attr = iota   // Zero value
	attrDup                // File may be removed
	attrJunk               // File and all of its copies may be removed
	attrKeep               // File must be preserved
	attrGone Attr = 1 << 7 // File no longer exists
)

const (
	attrDupS  = 'D'
	attrJunkS = 'J'
	attrKeepS = 'K'
	attrGoneS = 'X'
)

// IsDup returns whether this file is a duplicate that may be removed.
func (a Attr) IsDup() bool { return a&attrKeep == attrDup }

// IsJunk returns whether the file and all of its copies may be removed.
func (a Attr) IsJunk() bool { return a&attrKeep == attrJunk }

// Keep returns whether the file must be preserved.
func (a Attr) Keep() bool { return a&attrKeep == attrKeep }

// IsGone returns whether the file no longer exists.
func (a Attr) IsGone() bool { return a&attrGone != 0 }

// MayRemove returns whether the file may be removed.
func (a Attr) MayRemove() bool { return a&attrKeep == attrDup || a&attrKeep == attrJunk }

// String returns the string representation of the file attributes.
func (a Attr) String() string {
	if a == 0 {
		return ""
	}
	if a&^(attrKeep|attrGone) != 0 {
		panic(fmt.Sprintf("index: invalid attribute value: 0x%X", byte(a)))
	}
	b := make([]byte, 0, 2)
	switch a & attrKeep {
	case attrDup:
		b = append(b, attrDupS)
	case attrJunk:
		b = append(b, attrJunkS)
	case attrKeep:
		b = append(b, attrKeepS)
	}
	if a.IsGone() {
		b = append(b, attrGoneS)
	}
	return unsafe.String(&b[0], len(b))
}

// parseAttr decodes string attribute representation. It panics if the string is
// invalid.
func parseAttr(b []byte) (a Attr) {
	if len(b) == 0 {
		return
	}
	ok := len(b) == 1
	if b[len(b)-1] == attrGoneS {
		if a = attrGone; ok {
			return
		}
		ok = len(b) == 2
	}
	switch b[0] {
	case attrDupS:
		a |= attrDup
	case attrJunkS:
		a |= attrJunk
	case attrKeepS:
		a |= attrKeep
	default:
		ok = false
	}
	if ok {
		return
	}
	panic(fmt.Sprintf("index: invalid file attribute: %s", b))
}
