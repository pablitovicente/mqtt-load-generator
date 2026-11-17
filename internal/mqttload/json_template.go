package mqttload

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"slices"
	"text/template"

	"github.com/brianvoe/gofakeit/v7"
)

// RenderJSONTemplate renders a --json-template once with Go's text/template and gofakeit's
// functions ({{FirstName}}, {{Number 1 10}}), and returns the payload every client will send.
// Text from gofakeit functions is escaped for JSON, and the result must be valid JSON.
//
// gofakeit's own Template() isn't used: after rendering it turns every \n in the output into
// a line break (template.go in gofakeit v7.18.2), which would undo the escaping.
func RenderJSONTemplate(templateText string) ([]byte, error) {
	parsed, err := template.New("json-template").Funcs(templateFunctions(gofakeit.New(0))).Parse(templateText)
	if err != nil {
		return nil, err
	}

	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, nil); err != nil {
		return nil, err
	}

	// The payload never changes after this, so this one check covers every message sent.
	if !json.Valid(rendered.Bytes()) {
		return nil, fmt.Errorf("the rendered template is not valid JSON:\n%s", rendered.String())
	}

	return rendered.Bytes(), nil
}

// CheckJSONTemplateForBenchmark checks that a rendered --json-template can take the
// --benchmark timestamp: it must be a JSON object without a "timestamp" field of its own.
func CheckJSONTemplateForBenchmark(payload []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return fmt.Errorf("--benchmark adds a timestamp field, so the template must be a JSON object: %w", err)
	}

	if _, found := fields["timestamp"]; found {
		return fmt.Errorf(`the template already has a "timestamp" field, and --benchmark adds its own; don't use --benchmark with this template`)
	}

	return nil
}

// templateFunctions lists every gofakeit function a template can call, the way gofakeit's own
// templateFuncMap builds it (template.go): every Faker method that returns something, except
// the three gofakeit leaves out. Functions whose first result is text are wrapped so the text
// is escaped for use inside a JSON string. gofakeit's 12 template-only helpers (ToUpper,
// IntRange and the others) aren't included.
func templateFunctions(faker *gofakeit.Faker) template.FuncMap {
	excluded := []string{"RandomMapKey", "SQL", "Template"}

	stringType := reflect.TypeFor[string]()
	fakerValue := reflect.ValueOf(faker)
	fakerType := fakerValue.Type()
	functions := template.FuncMap{}

	for i := range fakerType.NumMethod() {
		name := fakerType.Method(i).Name
		method := fakerValue.Method(i)
		methodType := method.Type()

		if slices.Contains(excluded, name) || methodType.NumOut() == 0 {
			continue
		}

		if methodType.Out(0) != stringType {
			functions[name] = method.Interface()
			continue
		}

		// Reflection is slow, but this runs once at startup, not per message.
		escaped := reflect.MakeFunc(methodType, func(arguments []reflect.Value) []reflect.Value {
			var results []reflect.Value
			if methodType.IsVariadic() {
				results = method.CallSlice(arguments)
			} else {
				results = method.Call(arguments)
			}

			results[0] = reflect.ValueOf(escapeForJSONString(results[0].String()))
			return results
		})

		functions[name] = escaped.Interface()
	}

	return functions
}

// escapeForJSONString returns text escaped for use inside a JSON string, without the
// surrounding quotes: " becomes \", \ becomes \\, a line break becomes \n, and so on.
func escapeForJSONString(text string) string {
	// The only possible error is invalid UTF-8, which AppendQuote has already replaced with
	// the Unicode replacement character, so the result is valid either way.
	quoted, _ := jsontext.AppendQuote(make([]byte, 0, len(text)+2), text)

	return string(quoted[1 : len(quoted)-1])
}
