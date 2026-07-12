package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

type DataAssetCollectionDefinition struct {
	Dimensions []DataAssetCollectionDimension `json:"dimensions"`
}

type DataAssetCollectionDimension struct {
	Parameter string                    `json:"parameter"`
	Values    []any                     `json:"values,omitempty"`
	Range     *DataAssetCollectionRange `json:"range,omitempty"`
}

type DataAssetCollectionRange struct {
	From    int `json:"from"`
	Through int `json:"through"`
}

func (collection DataAssetCollectionDefinition) Validate(parameters map[string]DataParameterDefinition) error {
	_, err := collection.Cardinality(parameters)
	return err
}

func (collection DataAssetCollectionDefinition) Cardinality(parameters map[string]DataParameterDefinition) (uint64, error) {
	if len(collection.Dimensions) == 0 {
		return 0, fmt.Errorf("dimensions are required")
	}
	seen := make(map[string]struct{}, len(collection.Dimensions))
	total := uint64(1)
	for index, dimension := range collection.Dimensions {
		if err := validateDataName(dimension.Parameter, "collection dimension parameter"); err != nil {
			return 0, fmt.Errorf("dimension %d: %w", index, err)
		}
		parameter, ok := parameters[dimension.Parameter]
		if !ok {
			return 0, fmt.Errorf("dimension %d parameter %q is not defined", index, dimension.Parameter)
		}
		if _, duplicate := seen[dimension.Parameter]; duplicate {
			return 0, fmt.Errorf("dimension %d duplicates parameter %q", index, dimension.Parameter)
		}
		seen[dimension.Parameter] = struct{}{}

		count, err := dimension.cardinality(parameter)
		if err != nil {
			return 0, fmt.Errorf("dimension %d parameter %q: %w", index, dimension.Parameter, err)
		}
		if count == 0 {
			return 0, fmt.Errorf("dimension %d parameter %q has zero members", index, dimension.Parameter)
		}
		if total > math.MaxUint64/count {
			return 0, fmt.Errorf("collection cardinality overflow")
		}
		total *= count
	}
	return total, nil
}

func (dimension DataAssetCollectionDimension) DomainValues(parameter DataParameterDefinition) ([]any, error) {
	if dimension.Range != nil {
		count, err := dimension.Range.cardinality(parameter)
		if err != nil {
			return nil, err
		}
		maxInt := uint64(int(^uint(0) >> 1))
		if count > maxInt {
			return nil, fmt.Errorf("range is too large to expand")
		}
		values := make([]any, 0, int(count))
		for value := dimension.Range.From; value <= dimension.Range.Through; value++ {
			values = append(values, value)
		}
		return values, nil
	}
	if err := dimension.validateExplicitValues(parameter); err != nil {
		return nil, err
	}
	return append([]any(nil), dimension.Values...), nil
}

func (dimension DataAssetCollectionDimension) cardinality(parameter DataParameterDefinition) (uint64, error) {
	hasValues := dimension.Values != nil
	hasRange := dimension.Range != nil
	switch {
	case hasValues && hasRange:
		return 0, fmt.Errorf("must supply values or range, not both")
	case !hasValues && !hasRange:
		return 0, fmt.Errorf("must supply values or range")
	case hasValues:
		if err := dimension.validateExplicitValues(parameter); err != nil {
			return 0, err
		}
		return uint64(len(dimension.Values)), nil
	default:
		return dimension.Range.cardinality(parameter)
	}
}

func (dimension DataAssetCollectionDimension) validateExplicitValues(parameter DataParameterDefinition) error {
	if dimension.Range != nil {
		return fmt.Errorf("must supply values or range, not both")
	}
	if dimension.Values == nil {
		return fmt.Errorf("must supply values or range")
	}
	if len(dimension.Values) == 0 {
		return fmt.Errorf("values must not be empty")
	}
	seen := make(map[string]struct{}, len(dimension.Values))
	for index, value := range dimension.Values {
		key, err := collectionValueKey(value)
		if err != nil {
			return fmt.Errorf("values[%d]: %w", index, err)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("values[%d] duplicates an earlier value", index)
		}
		seen[key] = struct{}{}
		if err := validateCollectionValueType(parameter, value); err != nil {
			return fmt.Errorf("values[%d]: %w", index, err)
		}
	}
	return nil
}

func (dimension DataAssetCollectionDimension) validateRange(parameter DataParameterDefinition) error {
	if dimension.Values != nil {
		return fmt.Errorf("must supply values or range, not both")
	}
	if dimension.Range == nil {
		return fmt.Errorf("must supply values or range")
	}
	_, err := dimension.Range.cardinality(parameter)
	return err
}

func (rangeDefinition DataAssetCollectionRange) cardinality(parameter DataParameterDefinition) (uint64, error) {
	if parameter.Type != "int" {
		return 0, fmt.Errorf("range requires int parameter, got %q", parameter.Type)
	}
	if rangeDefinition.From > rangeDefinition.Through {
		return 0, fmt.Errorf("range from must be less than or equal to through")
	}
	count := uint64(rangeDefinition.Through) - uint64(rangeDefinition.From) + 1
	if count == 0 {
		return 0, fmt.Errorf("range cardinality overflow")
	}
	return count, nil
}

func validateCollectionValueType(parameter DataParameterDefinition, value any) error {
	switch parameter.Type {
	case "":
		_, err := collectionValueKey(value)
		return err
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("has type %T, want string", value)
		}
	case "int":
		if _, ok := value.(int); !ok {
			return fmt.Errorf("has type %T, want int", value)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("has type %T, want bool", value)
		}
	default:
		return fmt.Errorf("unsupported parameter type %q", parameter.Type)
	}
	return nil
}

func collectionValueKey(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return "string:" + typed, nil
	case int:
		return "int:" + strconv.Itoa(typed), nil
	case bool:
		if typed {
			return "bool:true", nil
		}
		return "bool:false", nil
	default:
		return "", fmt.Errorf("has type %T, want scalar string, int, or bool", value)
	}
}

func (dimension *DataAssetCollectionDimension) UnmarshalJSON(data []byte) error {
	var raw struct {
		Parameter string                    `json:"parameter"`
		Values    json.RawMessage           `json:"values"`
		Range     *DataAssetCollectionRange `json:"range"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	dimension.Parameter = raw.Parameter
	dimension.Range = raw.Range
	dimension.Values = nil
	if raw.Values == nil {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw.Values), []byte("null")) {
		return fmt.Errorf("collection dimension values must be an array")
	}
	var encodedValues []json.RawMessage
	if err := json.Unmarshal(raw.Values, &encodedValues); err != nil {
		return fmt.Errorf("collection dimension values must be an array: %w", err)
	}
	dimension.Values = make([]any, 0, len(encodedValues))
	for index, encodedValue := range encodedValues {
		value, err := unmarshalCollectionScalar(encodedValue)
		if err != nil {
			return fmt.Errorf("collection dimension values[%d]: %w", index, err)
		}
		dimension.Values = append(dimension.Values, value)
	}
	return nil
}

func unmarshalCollectionScalar(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	switch typed := value.(type) {
	case string, bool:
		return typed, nil
	case json.Number:
		integer, err := strconv.Atoi(typed.String())
		if err != nil {
			return nil, fmt.Errorf("number must be an int")
		}
		return integer, nil
	default:
		return nil, fmt.Errorf("must be a scalar string, int, or bool")
	}
}
