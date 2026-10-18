package csvsum

import (
	"encoding/csv"
	"io"
	"strconv"
)

// SumColumn adds up column col of a CSV with a header row.
func SumColumn(r io.Reader, col int) (float64, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return 0, err
	}
	total := 0.0
	for _, row := range rows[1:] {
		v, err := strconv.ParseFloat(row[col], 64)
		if err != nil {
			return 0, err
		}
		total += v
	}
	return total, nil
}
