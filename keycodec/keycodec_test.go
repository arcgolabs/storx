package keycodec_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/arcgolabs/storx/keycodec"
)

func TestBytesCodecClones(t *testing.T) {
	c := keycodec.Bytes()

	encoded, err := c.EncodeKey([]byte("abc"))
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	encoded[0] = 'z'

	decoded, err := c.DecodeKey([]byte("abc"))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	decoded[0] = 'x'

	if string(encoded) != "zbc" {
		t.Fatalf("unexpected encoded key: %q", string(encoded))
	}
	if string(decoded) != "xbc" {
		t.Fatalf("unexpected decoded key: %q", string(decoded))
	}
}

func TestUint64Ordering(t *testing.T) {
	c := keycodec.Uint64BE()

	a, err := c.EncodeKey(2)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	b, err := c.EncodeKey(10)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if bytes.Compare(a, b) >= 0 {
		t.Fatalf("expected 2 to sort before 10")
	}
}

func TestInt64Ordering(t *testing.T) {
	c := keycodec.Int64BE()

	a, err := c.EncodeKey(-5)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	b, err := c.EncodeKey(7)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if bytes.Compare(a, b) >= 0 {
		t.Fatalf("expected -5 to sort before 7")
	}
}

func TestTimeOrdering(t *testing.T) {
	c := keycodec.TimeUnixNanoBE()

	a, err := c.EncodeKey(time.Unix(0, 1).UTC())
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	b, err := c.EncodeKey(time.Unix(0, 2).UTC())
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if bytes.Compare(a, b) >= 0 {
		t.Fatalf("expected earlier time to sort first")
	}
}
