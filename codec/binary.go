package codec

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/internal/bytesx"
	"github.com/samber/oops"
)

type binaryCodec[T any] struct{}

// Binary returns a codec that delegates to encoding.BinaryMarshaler and
// encoding.BinaryUnmarshaler.
func Binary[T any]() Codec[T] {
	return binaryCodec[T]{}
}

func (binaryCodec[T]) Marshal(value T) ([]byte, error) {
	raw := any(value)
	if marshaler, ok := raw.(encoding.BinaryMarshaler); ok {
		v := reflect.ValueOf(raw)
		if v.Kind() == reflect.Pointer && v.IsNil() {
			return nil, oops.In("storx/codec").
				With("op", "binary_marshal", "type", codecTypeOf[T]()).
				Wrapf(errors.Join(storx.ErrCodec, storx.ErrInvalidValue), "binary marshaler is nil")
		}

		data, err := marshaler.MarshalBinary()
		if err != nil {
			return nil, oops.In("storx/codec").
				With("op", "binary_marshal", "type", codecTypeOf[T]()).
				Wrapf(errors.Join(storx.ErrCodec, err), "marshal value with binary codec")
		}
		return bytesx.Clone(data), nil
	}

	return nil, oops.In("storx/codec").
		With("op", "binary_marshal", "type", codecTypeOf[T]()).
		Wrapf(errors.Join(storx.ErrCodec, storx.ErrInvalidValue), "type %T does not implement encoding.BinaryMarshaler", value)
}

func (binaryCodec[T]) Unmarshal(data []byte) (T, error) {
	var zero T

	target, finalize, err := newBinaryUnmarshalTarget[T]()
	if err != nil {
		return zero, oops.In("storx/codec").
			With("op", "binary_unmarshal", "type", codecTypeOf[T]()).
			Wrapf(errors.Join(storx.ErrCodec, err), "allocate binary unmarshal target")
	}

	if err := target.UnmarshalBinary(bytesx.Clone(data)); err != nil {
		return zero, oops.In("storx/codec").
			With("op", "binary_unmarshal", "type", codecTypeOf[T](), "size", len(data)).
			Wrapf(errors.Join(storx.ErrCodec, err), "unmarshal value with binary codec")
	}

	return finalize(), nil
}

func codecTypeOf[T any]() string {
	return reflect.TypeOf((*T)(nil)).Elem().String()
}

func newBinaryUnmarshalTarget[T any]() (encoding.BinaryUnmarshaler, func() T, error) {
	targetType := reflect.TypeOf((*T)(nil)).Elem()
	if targetType.Kind() == reflect.Interface {
		return nil, nil, fmt.Errorf("type %s must be concrete", targetType)
	}

	if targetType.Kind() == reflect.Pointer {
		value := reflect.New(targetType.Elem())
		typedValue, ok := value.Interface().(T)
		if !ok {
			return nil, nil, fmt.Errorf("cannot allocate %s", targetType)
		}

		unmarshaler, ok := value.Interface().(encoding.BinaryUnmarshaler)
		if !ok {
			return nil, nil, fmt.Errorf("type %s does not implement encoding.BinaryUnmarshaler", targetType)
		}

		return unmarshaler, func() T { return typedValue }, nil
	}

	value := reflect.New(targetType)
	unmarshaler, ok := value.Interface().(encoding.BinaryUnmarshaler)
	if !ok {
		return nil, nil, fmt.Errorf("type %s does not implement encoding.BinaryUnmarshaler", targetType)
	}

	return unmarshaler, func() T {
		return value.Elem().Interface().(T)
	}, nil
}
