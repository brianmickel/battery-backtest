package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"battery-backtest/internal/config"
)

func decodeRequest(raw []byte, target reflect.Value) error {
	if !json.Valid(raw) {
		return invalid("", "Expected a JSON object.")
	}
	if err := validateRequestValue(raw, target.Type(), ""); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target.Addr().Interface()); err != nil {
		return invalid("", "Invalid request fields or types.")
	}
	return nil
}

// Only the two fixed request DTOs are traversed. Configuration keeps its own
// decoder; this boundary adds the request path and comparison identity.
func validateRequestValue(raw []byte, typ reflect.Type, path string) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return invalid(path, "Null values are not allowed; omit optional fields.")
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == reflect.TypeFor[config.Input]() {
		var input config.Input
		if err := json.Unmarshal(raw, &input); err != nil {
			var validation *config.ValidationError
			if errors.As(err, &validation) {
				return invalid(fieldPath(path, validation.Field), validation.Message)
			}
			return invalid(path, "Invalid configuration.")
		}
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		members, names, err := requestMembers(raw, path)
		if err != nil {
			return err
		}
		fields := make(map[string]reflect.Type, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fields[strings.Split(field.Tag.Get("json"), ",")[0]] = field.Type
		}
		var variation string
		if typ == reflect.TypeFor[variationRequest]() {
			if name, present := members["name"]; present {
				_ = json.Unmarshal(name, &variation)
			}
		}
		for _, name := range names {
			field, known := fields[name]
			if !known {
				err = invalid(fieldPath(path, name), "Unknown field.")
			} else {
				err = validateRequestValue(members[name], field, fieldPath(path, name))
			}
			if err != nil {
				if variation != "" {
					failure := classify(err)
					failure.variation = variation
					if strings.HasPrefix(failure.field, path+".config") {
						failure.field = strings.TrimPrefix(failure.field, path+".")
					}
					return failure
				}
				return err
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return invalid(path, "Must be an array.")
		}
		for i, value := range values {
			if err := validateRequestValue(value, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		if err := json.Unmarshal(raw, reflect.New(typ).Interface()); err != nil {
			return invalid(path, "Invalid field type.")
		}
	}
	return nil
}

func requestMembers(raw []byte, path string) (map[string]json.RawMessage, []string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, nil, invalid(path, "Must be an object.")
	}
	members := map[string]json.RawMessage{}
	var names []string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, invalid(path, "Invalid JSON object.")
		}
		name, ok := token.(string)
		if !ok {
			return nil, nil, invalid(path, "Invalid field name.")
		}
		if path == "" && name == "api_key" {
			return nil, nil, invalid("api_key", "Move the API key from JSON to X-GridStatus-API-Key.")
		}
		if _, exists := members[name]; exists {
			return nil, nil, invalid(fieldPath(path, name), "Duplicate field.")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, invalid(fieldPath(path, name), "Invalid field value.")
		}
		members[name] = value
		names = append(names, name)
	}
	return members, names, nil
}

func fieldPath(parent, field string) string {
	if parent == "" {
		return field
	}
	if field == "" {
		return parent
	}
	return parent + "." + field
}
