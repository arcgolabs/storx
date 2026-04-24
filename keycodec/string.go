package keycodec

type stringCodec struct{}

// String returns a codec for string keys.
func String() Codec[string] {
	return stringCodec{}
}

func (stringCodec) EncodeKey(key string) ([]byte, error) {
	return []byte(key), nil
}

func (stringCodec) DecodeKey(data []byte) (string, error) {
	return string(data), nil
}
