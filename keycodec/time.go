package keycodec

import "time"

type timeUnixNanoBECodec struct{}

// TimeUnixNanoBE returns a time codec ordered by UnixNano.
func TimeUnixNanoBE() Codec[time.Time] {
	return timeUnixNanoBECodec{}
}

func (timeUnixNanoBECodec) EncodeKey(key time.Time) ([]byte, error) {
	return Int64BE().EncodeKey(key.UnixNano())
}

func (timeUnixNanoBECodec) DecodeKey(data []byte) (time.Time, error) {
	nanos, err := Int64BE().DecodeKey(data)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, nanos).UTC(), nil
}
