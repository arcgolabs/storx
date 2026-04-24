// Package bytesx provides small byte-slice helpers shared across storx modules.
package bytesx

// Clone returns a detached copy of src.
func Clone(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}
