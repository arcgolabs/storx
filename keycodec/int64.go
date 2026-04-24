package keycodec

import (
	"encoding/binary"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/samber/oops"
)

const int64SignMask = uint64(1 << 63)

type int64BECodec struct{}

// Int64BE returns a big-endian int64 codec that preserves numeric ordering.
func Int64BE() Codec[int64] {
	return int64BECodec{}
}

func (int64BECodec) EncodeKey(key int64) ([]byte, error) {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(key)^int64SignMask)
	return data, nil
}

func (int64BECodec) DecodeKey(data []byte) (int64, error) {
	if len(data) != 8 {
		return 0, oops.In("storx/keycodec").
			With("op", "decode_int64_be", "size", len(data)).
			Wrapf(errors.Join(storx.ErrKeyCodec, storx.ErrInvalidKey), "int64 key requires 8 bytes")
	}
	return int64(binary.BigEndian.Uint64(data) ^ int64SignMask), nil
}
