package display

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteReport writes a pub or sub report to output as one JSON line.
func WriteReport(output io.Writer, report any) error {
	if err := json.NewEncoder(output).Encode(report); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	return nil
}
