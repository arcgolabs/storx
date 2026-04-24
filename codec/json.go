package codec

import (
	"encoding/json"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/samber/oops"
)

type jsonCodec[T any] struct{}

// JSON returns a codec backed by encoding/json.
func JSON[T any]() Codec[T] {
	return jsonCodec[T]{}
}

func (jsonCodec[T]) Marshal(value T) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, oops.In("storx/codec").
			With("op", "json_marshal", "type", codecTypeOf[T]()).
			Wrapf(errors.Join(storx.ErrCodec, err), "marshal value with json")
	}
	return data, nil
}

func (jsonCodec[T]) Unmarshal(data []byte) (T, error) {
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return value, oops.In("storx/codec").
			With("op", "json_unmarshal", "type", codecTypeOf[T](), "size", len(data)).
			Wrapf(errors.Join(storx.ErrCodec, err), "unmarshal value with json")
	}
	return value, nil
}
