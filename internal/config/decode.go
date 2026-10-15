package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError identifies a safe field path relative to the configuration.
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}
func invalid(field, message string) error { return &ValidationError{Field: field, Message: message} }

type inputWire Input

func (i *Input) UnmarshalJSON(raw []byte) error {
	if !json.Valid(raw) {
		return invalid("", "invalid JSON configuration")
	}
	var wire inputWire
	if err := decodeYAML(raw, &wire); err != nil {
		return err
	}
	if err := validateDecodedInput(Input(wire)); err != nil {
		return err
	}
	*i = Input(wire)
	return nil
}

func (i *Input) UnmarshalYAML(node *yaml.Node) error {
	var wire inputWire
	if err := decodeNode(node, &wire); err != nil {
		return err
	}
	if err := validateDecodedInput(Input(wire)); err != nil {
		return err
	}
	*i = Input(wire)
	return nil
}

func validateDecodedInput(i Input) error {
	if err := validateStrategyName(i.Strategy.Name); err != nil {
		return err
	}
	return compatibleParams(i.Strategy)
}

func decodeYAML(raw []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return invalid("", "invalid YAML configuration")
	}
	if len(node.Content) != 1 {
		return invalid("", "must be an object")
	}
	if err := decodeNode(node.Content[0], target); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return invalid("", "must contain exactly one document")
	}
	return nil
}

func decodeNode(node *yaml.Node, target any) error {
	if err := validateNode(node, reflect.TypeOf(target).Elem(), ""); err != nil {
		return err
	}
	if err := node.Decode(target); err != nil {
		return invalid("", "invalid configuration value")
	}
	return nil
}

// The structural pass prevents YAML coercions, null-as-omission and unknown fields.
// Reflection uses only the fixed patch structs; decoded maps never escape this boundary.
func validateNode(node *yaml.Node, typ reflect.Type, path string) error {
	if node.Tag == "!!null" {
		return invalid(path, "null is not allowed")
	}
	if node.Kind == yaml.AliasNode {
		return invalid(path, "aliases are not allowed")
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return invalid(path, "must be an object")
		}
		fields := make(map[string]reflect.Type, typ.NumField())
		for idx := 0; idx < typ.NumField(); idx++ {
			field := typ.Field(idx)
			fields[strings.Split(field.Tag.Get("yaml"), ",")[0]] = field.Type
		}
		seen := make(map[string]bool, len(node.Content)/2)
		for idx := 0; idx < len(node.Content); idx += 2 {
			key, value := node.Content[idx], node.Content[idx+1]
			childPath := key.Value
			if path != "" {
				childPath = path + "." + childPath
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return invalid(path, "field names must be strings")
			}
			fieldType, known := fields[key.Value]
			if !known {
				return invalid(childPath, "unknown field")
			}
			if seen[key.Value] {
				return invalid(childPath, "duplicate field")
			}
			seen[key.Value] = true
			if err := validateNode(value, fieldType, childPath); err != nil {
				return err
			}
			if childPath == "battery_file" && value.Value == "" {
				return invalid(childPath, "must not be empty")
			}
		}
	case reflect.String:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return invalid(path, "must be a string")
		}
	case reflect.Int:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			return invalid(path, "must be an integer")
		}
		var value int
		if err := node.Decode(&value); err != nil {
			return invalid(path, "integer is out of range")
		}
	case reflect.Float64:
		if node.Kind != yaml.ScalarNode || (node.Tag != "!!float" && node.Tag != "!!int") {
			return invalid(path, "must be a number")
		}
		var value float64
		if err := node.Decode(&value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return invalid(path, "must be finite")
		}
	default:
		return fmt.Errorf("unsupported configuration field type %s", typ)
	}
	return nil
}
