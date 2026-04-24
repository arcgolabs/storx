package keycodec

// Codec converts typed keys to and from their byte representation.
type Codec[K any] interface {
	EncodeKey(key K) ([]byte, error)
	DecodeKey(data []byte) (K, error)
}
