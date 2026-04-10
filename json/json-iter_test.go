package json

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalUnmarshal(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	original := sample{Name: "Alice", Age: 30}
	data, err := Marshal(original)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"name":"Alice"`)

	var decoded sample
	err = Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestMarshal_Nil(t *testing.T) {
	data, err := Marshal(nil)
	assert.NoError(t, err)
	assert.Equal(t, "null", string(data))
}

func TestUnmarshal_InvalidJSON(t *testing.T) {
	var result map[string]any
	err := Unmarshal([]byte(`{invalid}`), &result)
	assert.Error(t, err)
}

func TestNewEncoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	assert.NotNil(t, enc)

	err := enc.Encode(map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), `"key":"value"`)
}

func TestNewDecoder(t *testing.T) {
	r := strings.NewReader(`{"key":"value"}`)
	dec := NewDecoder(r)
	assert.NotNil(t, dec)

	var result map[string]string
	err := dec.Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

func TestRawMessage(t *testing.T) {
	var raw RawMessage = []byte(`{"nested":true}`)
	assert.NotEmpty(t, raw)
}
