// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package report

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSON writes any value as indented JSON to w.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encoding JSON output: %w", err)
	}
	return nil
}
