// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package validation provides validation utilities for checking input values.
package validation

import (
	"errors"
	"fmt"
)

var ErrInvalidSize = errors.New("invalid buffer size")

func IsPowerOfTwo(size uint16) error {
	for i := range 16 {
		if size == 1<<i {
			return nil
		}
	}

	// Only build allowedSizes if we actually need the error
	allowedSizes := make([]uint16, 0)
	for i := range 16 {
		allowedSizes = append(allowedSizes, 1<<i)
	}

	return fmt.Errorf("%w: %d is not a valid size, allowed sizes: %v", ErrInvalidSize, size, allowedSizes)
}
