package keycodec

import (
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/internal/bytesx"
	"github.com/samber/oops"
)

// Component encodes and decodes one field of a composite key.
type Component[K any] interface {
	encode(key K) ([]byte, error)
	decode(data []byte, target *K) error
}

type component[K any] struct {
	encodeFn func(key K) ([]byte, error)
	decodeFn func(data []byte, target *K) error
}

func (c component[K]) encode(key K) ([]byte, error) {
	return c.encodeFn(key)
}

func (c component[K]) decode(data []byte, target *K) error {
	return c.decodeFn(data, target)
}

// Field adapts a plain key codec into one component of a composite key.
func Field[K any, T any](
	codec Codec[T],
	get func(key K) T,
	set func(target *K, value T),
) Component[K] {
	return component[K]{
		encodeFn: func(key K) ([]byte, error) {
			if codec == nil {
				return nil, invalidCompositeKey("encode_composite_field", "field codec is nil")
			}
			if get == nil {
				return nil, invalidCompositeKey("encode_composite_field", "field getter is nil")
			}
			data, err := codec.EncodeKey(get(key))
			if err != nil {
				return nil, wrapCompositeError("encode_composite_field", err, "encode composite field")
			}
			return bytesx.Clone(data), nil
		},
		decodeFn: func(data []byte, target *K) error {
			if codec == nil {
				return invalidCompositeKey("decode_composite_field", "field codec is nil")
			}
			if set == nil {
				return invalidCompositeKey("decode_composite_field", "field setter is nil")
			}
			value, err := codec.DecodeKey(bytesx.Clone(data))
			if err != nil {
				return wrapCompositeError("decode_composite_field", err, "decode composite field")
			}
			set(target, value)
			return nil
		},
	}
}

type compositeCodec[K any] struct {
	components []Component[K]
}

// Composite returns a lexicographically ordered tuple codec built from field
// components.
func Composite[K any](components ...Component[K]) Codec[K] {
	return compositeCodec[K]{
		components: append([]Component[K](nil), components...),
	}
}

// ComponentPrefix returns the encoded prefix for one composite key component.
func ComponentPrefix[T any](codec Codec[T], value T) ([]byte, error) {
	if codec == nil {
		return nil, invalidCompositeKey("encode_composite_prefix", "field codec is nil")
	}
	data, err := codec.EncodeKey(value)
	if err != nil {
		return nil, wrapCompositeError("encode_composite_prefix", err, "encode composite prefix")
	}
	return appendCompositePart(nil, bytesx.Clone(data)), nil
}

func (c compositeCodec[K]) EncodeKey(key K) ([]byte, error) {
	if len(c.components) == 0 {
		return nil, invalidCompositeKey("encode_composite", "composite key requires at least one component")
	}

	var encoded []byte
	for _, part := range c.components {
		if part == nil {
			return nil, invalidCompositeKey("encode_composite", "composite component is nil")
		}
		data, err := part.encode(key)
		if err != nil {
			return nil, err
		}
		encoded = appendCompositePart(encoded, data)
	}
	return encoded, nil
}

func (c compositeCodec[K]) DecodeKey(data []byte) (K, error) {
	var key K

	if len(c.components) == 0 {
		return key, invalidCompositeKey("decode_composite", "composite key requires at least one component")
	}

	remaining := data
	for _, part := range c.components {
		if part == nil {
			return key, invalidCompositeKey("decode_composite", "composite component is nil")
		}

		componentData, tail, err := nextCompositePart(remaining)
		if err != nil {
			return key, err
		}
		if err := part.decode(componentData, &key); err != nil {
			return key, err
		}
		remaining = tail
	}

	if len(remaining) > 0 {
		return key, invalidCompositeKey("decode_composite", "unexpected trailing composite key bytes")
	}
	return key, nil
}

func appendCompositePart(dst, data []byte) []byte {
	for _, b := range data {
		if b == 0 {
			dst = append(dst, 0, 0xff)
			continue
		}
		dst = append(dst, b)
	}
	return append(dst, 0, 0)
}

func nextCompositePart(data []byte) ([]byte, []byte, error) {
	component := make([]byte, 0, len(data))

	for i := 0; i < len(data); i++ {
		if data[i] != 0 {
			component = append(component, data[i])
			continue
		}

		if i+1 >= len(data) {
			return nil, nil, invalidCompositeKey("decode_composite", "truncated composite key escape sequence")
		}

		switch data[i+1] {
		case 0:
			return component, data[i+2:], nil
		case 0xff:
			component = append(component, 0)
			i++
		default:
			return nil, nil, invalidCompositeKey("decode_composite", "invalid composite key escape sequence")
		}
	}

	return nil, nil, invalidCompositeKey("decode_composite", "missing composite key terminator")
}

func invalidCompositeKey(op, format string, attrs ...any) error {
	builder := oops.In("storx/keycodec").With("op", op)
	if len(attrs) > 0 {
		builder = builder.With(attrs...)
	}
	return builder.Wrapf(errors.Join(storx.ErrKeyCodec, storx.ErrInvalidKey), "%s", format)
}

func wrapCompositeError(op string, err error, format string, attrs ...any) error {
	builder := oops.In("storx/keycodec").With("op", op)
	if len(attrs) > 0 {
		builder = builder.With(attrs...)
	}
	return builder.Wrapf(errors.Join(storx.ErrKeyCodec, err), "%s", format)
}
