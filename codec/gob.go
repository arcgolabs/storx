package codec

import (
	"bytes"
	"encoding/gob"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/samber/oops"
)

type gobCodec[T any] struct{}

// Gob returns a codec backed by encoding/gob.
func Gob[T any]() Codec[T] {
	return gobCodec[T]{}
}

func (gobCodec[T]) Marshal(value T) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return nil, oops.In("storx/codec").
			With("op", "gob_marshal", "type", codecTypeOf[T]()).
			Wrapf(errors.Join(storx.ErrCodec, err), "marshal value with gob")
	}
	return buf.Bytes(), nil
}

func (gobCodec[T]) Unmarshal(data []byte) (T, error) {
	var value T
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&value); err != nil {
		return value, oops.In("storx/codec").
			With("op", "gob_unmarshal", "type", codecTypeOf[T](), "size", len(data)).
			Wrapf(errors.Join(storx.ErrCodec, err), "unmarshal value with gob")
	}
	return value, nil
}
