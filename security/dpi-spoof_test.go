package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRandDuration_Zero(t *testing.T) {
	d := randDuration(0)
	assert.Equal(t, time.Duration(0), d)
}

func TestRandDuration_Negative(t *testing.T) {
	d := randDuration(-time.Second)
	assert.Equal(t, time.Duration(0), d)
}

func TestRandDuration_Positive(t *testing.T) {
	maximum := 100 * time.Millisecond
	for range 100 {
		d := randDuration(maximum)
		assert.True(t, d >= 0, "duration should be non-negative")
		assert.True(t, d < maximum, "duration should be less than max")
	}
}
