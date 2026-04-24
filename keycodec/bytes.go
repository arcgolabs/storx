package keycodec

import "github.com/arcgolabs/storx/internal/bytesx"

type bytesCodec struct{}

// Bytes returns a codec for []byte keys.
func Bytes() Codec[[]byte] {
	return bytesCodec{}
}

func (bytesCodec) EncodeKey(key []byte) ([]byte, error) {
	return bytesx.Clone(key), nil
}

func (bytesCodec) DecodeKey(data []byte) ([]byte, error) {
	return bytesx.Clone(data), nil
}
