package codec

import (
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/internal/bytesx"
	"github.com/samber/oops"
)

type bytesCodec struct{}

// Bytes returns a codec for []byte that always detaches from the input slice.
func Bytes() Codec[[]byte] {
	return bytesCodec{}
}

func (bytesCodec) Marshal(value []byte) ([]byte, error) {
	return bytesx.Clone(value), nil
}

func (bytesCodec) Unmarshal(data []byte) ([]byte, error) {
	return bytesx.Clone(data), nil
}

// BytesNoCopy is intentionally unsupported in the first version to keep the API
// memory-safe by default.
func BytesNoCopy() Codec[[]byte] {
	return unsupportedBytesNoCopyCodec{}
}

type unsupportedBytesNoCopyCodec struct{}

func (unsupportedBytesNoCopyCodec) Marshal(_ []byte) ([]byte, error) {
	return nil, oops.In("storx/codec").
		With("op", "bytes_no_copy_marshal").
		Wrapf(errors.Join(storx.ErrCodec, storx.ErrInvalidValue), "BytesNoCopy is not implemented")
}

func (unsupportedBytesNoCopyCodec) Unmarshal(_ []byte) ([]byte, error) {
	return nil, oops.In("storx/codec").
		With("op", "bytes_no_copy_unmarshal").
		Wrapf(errors.Join(storx.ErrCodec, storx.ErrInvalidValue), "BytesNoCopy is not implemented")
}
