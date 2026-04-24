package keycodec

import (
	"encoding/binary"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/samber/oops"
)

type uint64BECodec struct{}

// Uint64BE returns a big-endian uint64 codec that preserves numeric ordering.
func Uint64BE() Codec[uint64] {
	return uint64BECodec{}
}

func (uint64BECodec) EncodeKey(key uint64) ([]byte, error) {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, key)
	return data, nil
}

func (uint64BECodec) DecodeKey(data []byte) (uint64, error) {
	if len(data) != 8 {
		return 0, oops.In("storx/keycodec").
			With("op", "decode_uint64_be", "size", len(data)).
			Wrapf(errors.Join(storx.ErrKeyCodec, storx.ErrInvalidKey), "uint64 key requires 8 bytes")
	}
	return binary.BigEndian.Uint64(data), nil
}
