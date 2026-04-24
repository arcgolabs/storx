package codec

type stringCodec struct{}

// String returns a codec for UTF-8 strings.
func String() Codec[string] {
	return stringCodec{}
}

func (stringCodec) Marshal(value string) ([]byte, error) {
	return []byte(value), nil
}

func (stringCodec) Unmarshal(data []byte) (string, error) {
	return string(data), nil
}
