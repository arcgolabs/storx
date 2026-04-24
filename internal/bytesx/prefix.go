package bytesx

// PrefixSuccessor returns the smallest byte slice that is strictly greater than
// every byte slice having prefix as a prefix. When no such successor exists, it
// returns nil.
func PrefixSuccessor(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}

	next := Clone(prefix)
	for index := len(next) - 1; index >= 0; index-- {
		if next[index] != 0xFF {
			next[index]++
			return next[:index+1]
		}
	}

	return nil
}
