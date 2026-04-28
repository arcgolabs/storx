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

type compositeUserKey struct {
	TenantID string
	UserID   uint64
}

func TestCompositeRoundTrip(t *testing.T) {
	c := keycodec.Composite(
		keycodec.Field(
			keycodec.String(),
			func(key compositeUserKey) string { return key.TenantID },
			func(target *compositeUserKey, value string) { target.TenantID = value },
		),
		keycodec.Field(
			keycodec.Uint64BE(),
			func(key compositeUserKey) uint64 { return key.UserID },
			func(target *compositeUserKey, value uint64) { target.UserID = value },
		),
	)

	encoded, err := c.EncodeKey(compositeUserKey{
		TenantID: "tenant-a",
		UserID:   42,
	})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := c.DecodeKey(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded.TenantID != "tenant-a" || decoded.UserID != 42 {
		t.Fatalf("unexpected decoded key: %#v", decoded)
	}
}

func TestCompositeOrdering(t *testing.T) {
	c := keycodec.Composite(
		keycodec.Field(
			keycodec.String(),
			func(key compositeUserKey) string { return key.TenantID },
			func(target *compositeUserKey, value string) { target.TenantID = value },
		),
		keycodec.Field(
			keycodec.Uint64BE(),
			func(key compositeUserKey) uint64 { return key.UserID },
			func(target *compositeUserKey, value uint64) { target.UserID = value },
		),
	)

	a, err := c.EncodeKey(compositeUserKey{TenantID: "tenant-a", UserID: 2})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	b, err := c.EncodeKey(compositeUserKey{TenantID: "tenant-a", UserID: 10})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	crossTenant, err := c.EncodeKey(compositeUserKey{TenantID: "tenant-b", UserID: 1})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if bytes.Compare(a, b) >= 0 {
		t.Fatalf("expected lower second field to sort first")
	}
	if bytes.Compare(b, crossTenant) >= 0 {
		t.Fatalf("expected tenant-a to sort before tenant-b")
	}
}

func TestCompositeEscapesZeroBytes(t *testing.T) {
	type byteKey struct {
		Scope []byte
		Name  string
	}

	c := keycodec.Composite(
		keycodec.Field(
			keycodec.Bytes(),
			func(key byteKey) []byte { return key.Scope },
			func(target *byteKey, value []byte) { target.Scope = value },
		),
		keycodec.Field(
			keycodec.String(),
			func(key byteKey) string { return key.Name },
			func(target *byteKey, value string) { target.Name = value },
		),
	)

	encoded, err := c.EncodeKey(byteKey{
		Scope: []byte{0, 1, 0},
		Name:  "alpha",
	})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := c.DecodeKey(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !bytes.Equal(decoded.Scope, []byte{0, 1, 0}) || decoded.Name != "alpha" {
		t.Fatalf("unexpected decoded key: %#v", decoded)
	}
}
