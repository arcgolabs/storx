package codec_test

import (
	"encoding/binary"
	"testing"

	"github.com/arcgolabs/storx/codec"
)

type user struct {
	ID   string
	Name string
}

type binaryValue struct {
	ID uint64
}

func (v binaryValue) MarshalBinary() ([]byte, error) {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, v.ID)
	return data, nil
}

func (v *binaryValue) UnmarshalBinary(data []byte) error {
	v.ID = binary.BigEndian.Uint64(data)
	return nil
}

func TestJSONRoundTrip(t *testing.T) {
	c := codec.JSON[user]()

	data, err := c.Marshal(user{ID: "u1", Name: "alice"})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	got, err := c.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.ID != "u1" || got.Name != "alice" {
		t.Fatalf("unexpected value: %#v", got)
	}
}

func TestBytesCodecClones(t *testing.T) {
	c := codec.Bytes()

	encoded, err := c.Marshal([]byte("abc"))
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	encoded[0] = 'z'

	decoded, err := c.Unmarshal([]byte("abc"))
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	decoded[0] = 'x'

	if string(encoded) != "zbc" {
		t.Fatalf("unexpected encoded data: %q", string(encoded))
	}
	if string(decoded) != "xbc" {
		t.Fatalf("unexpected decoded data: %q", string(decoded))
	}
}

func TestGobRoundTrip(t *testing.T) {
	c := codec.Gob[user]()

	data, err := c.Marshal(user{ID: "u2", Name: "bob"})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	got, err := c.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.ID != "u2" || got.Name != "bob" {
		t.Fatalf("unexpected value: %#v", got)
	}
}

func TestBinaryRoundTrip(t *testing.T) {
	c := codec.Binary[*binaryValue]()

	data, err := c.Marshal(&binaryValue{ID: 42})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	got, err := c.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got == nil || got.ID != 42 {
		t.Fatalf("unexpected value: %#v", got)
	}
}
