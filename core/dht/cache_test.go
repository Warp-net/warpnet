package dht

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLRU(t *testing.T) {
	c := newLRU()
	assert.NotNil(t, c)
	assert.Equal(t, 0, c.Len())
}

func TestCache_AddAndGet(t *testing.T) {
	c := newLRU()
	evicted := c.Add("key1", "value1")
	assert.False(t, evicted)

	val, ok := c.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)
}

func TestCache_Contains(t *testing.T) {
	c := newLRU()
	assert.False(t, c.Contains("missing"))

	c.Add("key1", "value1")
	assert.True(t, c.Contains("key1"))
}

func TestCache_Peek(t *testing.T) {
	c := newLRU()
	c.Add("key1", "value1")

	val, ok := c.Peek("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)

	val, ok = c.Peek("missing")
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestCache_Remove(t *testing.T) {
	c := newLRU()
	c.Add("key1", "value1")
	assert.True(t, c.Contains("key1"))

	removed := c.Remove("key1")
	assert.True(t, removed)
	assert.False(t, c.Contains("key1"))
}

func TestCache_RemoveOldest(t *testing.T) {
	c := newLRU()
	c.Add("k1", "v1")
	c.Add("k2", "v2")

	k, v, ok := c.RemoveOldest()
	assert.True(t, ok)
	assert.Equal(t, "k1", k)
	assert.Equal(t, "v1", v)
}

func TestCache_RemoveOldest_Empty(t *testing.T) {
	c := newLRU()
	k, v, ok := c.RemoveOldest()
	assert.False(t, ok)
	assert.Nil(t, k)
	assert.Nil(t, v)
}

func TestCache_GetOldest(t *testing.T) {
	c := newLRU()
	c.Add("k1", "v1")
	c.Add("k2", "v2")

	k, v, ok := c.GetOldest()
	assert.True(t, ok)
	assert.Equal(t, "k1", k)
	assert.Equal(t, "v1", v)
}

func TestCache_GetOldest_Empty(t *testing.T) {
	c := newLRU()
	k, v, ok := c.GetOldest()
	assert.False(t, ok)
	assert.Nil(t, k)
	assert.Nil(t, v)
}

func TestCache_Keys(t *testing.T) {
	c := newLRU()
	c.Add("k1", "v1")
	c.Add("k2", "v2")

	keys := c.Keys()
	assert.Len(t, keys, 2)
}

func TestCache_Len(t *testing.T) {
	c := newLRU()
	assert.Equal(t, 0, c.Len())

	c.Add("k1", "v1")
	assert.Equal(t, 1, c.Len())

	c.Add("k2", "v2")
	assert.Equal(t, 2, c.Len())
}

func TestCache_Purge(t *testing.T) {
	c := newLRU()
	c.Add("k1", "v1")
	c.Add("k2", "v2")
	assert.Equal(t, 2, c.Len())

	c.Purge()
	assert.Equal(t, 0, c.Len())
}

func TestCache_Resize(t *testing.T) {
	c := newLRU()
	for i := range 10 {
		c.Add(i, i)
	}
	evicted := c.Resize(5)
	assert.True(t, evicted >= 0)
}

func TestCastKeyToString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"bytes", []byte("world"), "world"},
		{"runes", []rune("test"), "test"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
		{"int", int(7), "7"},
		{"int8", int8(8), "8"},
		{"int16", int16(16), "16"},
		{"int32", int32(32), "32"},
		{"int64", int64(42), "42"},
		{"uint", uint(5), "5"},
		{"uint8", uint8(8), "8"},
		{"uint16", uint16(16), "16"},
		{"uint32", uint32(32), "32"},
		{"uint64", uint64(99), "99"},
		{"float32", float32(1.5), "1.5"},
		{"float64", float64(3.14), "3.14"},
		{"struct", struct{ X int }{1}, "{1}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := castKeyToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCache_ByteKey(t *testing.T) {
	c := newLRU()
	c.Add([]byte("bytekey"), "val")

	val, ok := c.Get([]byte("bytekey"))
	assert.True(t, ok)
	assert.Equal(t, "val", val)
}

func TestCache_BoolKey(t *testing.T) {
	c := newLRU()
	c.Add(true, "yes")

	val, ok := c.Get(true)
	assert.True(t, ok)
	assert.Equal(t, "yes", val)
}
