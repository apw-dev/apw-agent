package csvsum

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// SumColumn adds up column col of a CSV with a header row.
func SumColumn(r io.Reader, col int) (float64, error) {
	if col < 0 {
		return 0, errors.New("negative column")
	}
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return 0, err
	}
	total := 0.0
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if col >= len(row) {
			return 0, fmt.Errorf("line %d: row has %d columns, need %d", i+1, len(row), col+1)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(row[col]), 64)
		if err != nil {
			return 0, fmt.Errorf("line %d: %w", i+1, err)
		}
		total += v
	}
	return total, nil
}
