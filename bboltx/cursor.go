package bboltx

import (
	"context"

	"go.etcd.io/bbolt"
)

type Cursor[K any, V any] interface {
	First() (K, V, bool, error)
	Last() (K, V, bool, error)
	Seek(prefix []byte) (K, V, bool, error)
	Next() (K, V, bool, error)
	Prev() (K, V, bool, error)
}

type cursor[K any, V any] struct {
	ctx    context.Context
	owner  *Bucket[K, V]
	cursor *bbolt.Cursor
	err    error
}

func (c *cursor[K, V]) First() (K, V, bool, error) {
	return c.readForward("cursor_first", func() ([]byte, []byte) {
		return c.cursor.First()
	})
}

func (c *cursor[K, V]) Last() (K, V, bool, error) {
	return c.readBackward("cursor_last", func() ([]byte, []byte) {
		return c.cursor.Last()
	})
}

func (c *cursor[K, V]) Seek(prefix []byte) (K, V, bool, error) {
	var zeroK K
	var zeroV V

	if err := c.validate(); err != nil {
		return zeroK, zeroV, false, err
	}
	if c.cursor == nil {
		return zeroK, zeroV, false, nil
	}

	rawKey, rawValue := c.cursor.Seek(prefix)
	for rawKey != nil && rawValue == nil {
		rawKey, rawValue = c.cursor.Next()
	}
	return c.decode("cursor_seek", rawKey, rawValue)
}

func (c *cursor[K, V]) Next() (K, V, bool, error) {
	return c.readForward("cursor_next", func() ([]byte, []byte) {
		return c.cursor.Next()
	})
}

func (c *cursor[K, V]) Prev() (K, V, bool, error) {
	return c.readBackward("cursor_prev", func() ([]byte, []byte) {
		return c.cursor.Prev()
	})
}

func (c *cursor[K, V]) validate() error {
	if c.err != nil {
		return c.err
	}
	if c.ctx != nil {
		if err := c.ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (c *cursor[K, V]) readForward(op string, step func() ([]byte, []byte)) (K, V, bool, error) {
	var zeroK K
	var zeroV V

	if err := c.validate(); err != nil {
		return zeroK, zeroV, false, err
	}
	if c.cursor == nil {
		return zeroK, zeroV, false, nil
	}

	rawKey, rawValue := step()
	for rawKey != nil && rawValue == nil {
		rawKey, rawValue = c.cursor.Next()
	}
	return c.decode(op, rawKey, rawValue)
}

func (c *cursor[K, V]) readBackward(op string, step func() ([]byte, []byte)) (K, V, bool, error) {
	var zeroK K
	var zeroV V

	if err := c.validate(); err != nil {
		return zeroK, zeroV, false, err
	}
	if c.cursor == nil {
		return zeroK, zeroV, false, nil
	}

	rawKey, rawValue := step()
	for rawKey != nil && rawValue == nil {
		rawKey, rawValue = c.cursor.Prev()
	}
	return c.decode(op, rawKey, rawValue)
}

func (c *cursor[K, V]) decode(op string, rawKey, rawValue []byte) (K, V, bool, error) {
	var zeroK K
	var zeroV V

	if rawKey == nil {
		return zeroK, zeroV, false, nil
	}

	key, err := c.owner.decodeKey(op, rawKey)
	if err != nil {
		return zeroK, zeroV, false, err
	}
	value, err := c.owner.decodeValue(op, rawValue)
	if err != nil {
		return zeroK, zeroV, false, err
	}
	return key, value, true, nil
}
