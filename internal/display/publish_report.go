package display

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pablitovicente/mqtt-load-generator/v2/internal/mqttload"
)

// WritePublishReport writes the report to output as one JSON line.
func WritePublishReport(output io.Writer, report mqttload.PublishReport) error {
	if err := json.NewEncoder(output).Encode(report); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	return nil
}
