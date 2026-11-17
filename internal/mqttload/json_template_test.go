package mqttload

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJSONTemplate_FillsValuesAndIsValidJSON(t *testing.T) {
	payload, err := RenderJSONTemplate(`{"name": "{{FirstName}}", "battery": {{Number 0 100}}, "online": {{Bool}}}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var decoded struct {
		Name    string `json:"name"`
		Battery int    `json:"battery"`
		Online  bool   `json:"online"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v, payload: %s", err, payload)
	}

	if decoded.Name == "" {
		t.Errorf("expected a generated name, got an empty string")
	}
	if decoded.Battery < 0 || decoded.Battery > 100 {
		t.Errorf("battery = %d, want 0..100", decoded.Battery)
	}
}

// TestRenderJSONTemplate_EscapesText checks the escaping with two gofakeit functions that
// break JSON when not escaped: Quote wraps its text in double quotes, and LoremIpsumParagraph
// here puts a line break between paragraphs.
func TestRenderJSONTemplate_EscapesText(t *testing.T) {
	payload, err := RenderJSONTemplate(`{"quote": "{{Quote}}", "text": "{{LoremIpsumParagraph 2 2 3 "\n"}}"}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var decoded struct {
		Quote string `json:"quote"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v, payload: %s", err, payload)
	}

	if !strings.HasPrefix(decoded.Quote, `"`) {
		t.Errorf("expected the quote to keep its double quotes after decoding, got %q", decoded.Quote)
	}
	if !strings.Contains(decoded.Text, "\n") {
		t.Errorf("expected the text to keep its line break after decoding, got %q", decoded.Text)
	}
}

// TestRenderJSONTemplate_KeepsUsersEscapes checks that a \n the user writes inside a JSON
// string stays an escape, so the payload decodes to a line break.
func TestRenderJSONTemplate_KeepsUsersEscapes(t *testing.T) {
	payload, err := RenderJSONTemplate(`{"text": "line one\nline two {{FirstName}}"}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var decoded struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v, payload: %s", err, payload)
	}

	if !strings.HasPrefix(decoded.Text, "line one\nline two ") {
		t.Errorf("text = %q, want it to start with %q", decoded.Text, "line one\nline two ")
	}
}

// TestRenderJSONTemplate_OptionalArguments checks a gofakeit text function with optional
// arguments (Sentence(wordCount ...int)), called with and without them.
func TestRenderJSONTemplate_OptionalArguments(t *testing.T) {
	payload, err := RenderJSONTemplate(`{"short": "{{Sentence}}", "five": "{{Sentence 5}}"}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !json.Valid(payload) {
		t.Errorf("payload is not valid JSON: %s", payload)
	}
}

func TestRenderJSONTemplate_Errors(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		wantInError string
	}{
		{"unknown function", `{"name": "{{Firstnme}}"}`, `function "Firstnme" not defined`},
		{"not JSON", `{"name": {{FirstName}}}`, "not valid JSON"},
		{"function that returns an object", `{"address": {{Address}}}`, "not valid JSON"},
		{"gofakeit template helper", `{"name": "{{ToUpper "a"}}"}`, `function "ToUpper" not defined`},
		{"broken template", `{"name": {{FirstName`, "unclosed action"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RenderJSONTemplate(tt.template)
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.wantInError) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantInError)
			}
		})
	}
}

func TestCheckJSONTemplateForBenchmark(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantInError string
	}{
		{"object", `{"name": "Halle"}`, ""},
		{"empty object", `{}`, ""},
		{"array", `[1, 2]`, "must be a JSON object"},
		{"has a timestamp field", `{"timestamp": 5, "name": "Halle"}`, "don't use --benchmark with this template"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckJSONTemplateForBenchmark([]byte(tt.payload))

			if tt.wantInError == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.wantInError) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantInError)
			}
		})
	}
}
