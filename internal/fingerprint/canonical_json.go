package fingerprint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const sha256HexLength = sha256.Size * 2

func CanonicalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func CanonicalJSONSHA256(value any) ([]byte, string, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return nil, "", err
	}
	return canonical, SHA256Hex(canonical), nil
}

func ValidateSHA256Hex(value string) error {
	if len(value) != sha256HexLength {
		return fmt.Errorf("sha256 hex must be %d characters", sha256HexLength)
	}
	for _, char := range value {
		if !('0' <= char && char <= '9') && !('a' <= char && char <= 'f') {
			return fmt.Errorf("sha256 hex must contain only lowercase hexadecimal characters")
		}
	}
	return nil
}

func writeCanonicalJSON(buf *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if typed {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	case string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Errorf("encode string: %w", err)
		}
		buf.Write(encoded)
		return nil
	case json.Number:
		text := typed.String()
		if !isCanonicalIntegerText(text) {
			return fmt.Errorf("canonical JSON v1 accepts only integer numbers")
		}
		buf.WriteString(text)
		return nil
	case int:
		buf.WriteString(strconv.FormatInt(int64(typed), 10))
		return nil
	case int8:
		buf.WriteString(strconv.FormatInt(int64(typed), 10))
		return nil
	case int16:
		buf.WriteString(strconv.FormatInt(int64(typed), 10))
		return nil
	case int32:
		buf.WriteString(strconv.FormatInt(int64(typed), 10))
		return nil
	case int64:
		buf.WriteString(strconv.FormatInt(typed, 10))
		return nil
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(typed), 10))
		return nil
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(typed), 10))
		return nil
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(typed), 10))
		return nil
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(typed), 10))
		return nil
	case uint64:
		buf.WriteString(strconv.FormatUint(typed, 10))
		return nil
	case float32, float64:
		return fmt.Errorf("canonical JSON v1 rejects floating-point numbers; use schema-defined strings for decimals")
	case []any:
		return writeCanonicalList(buf, typed)
	case map[string]any:
		return writeCanonicalObject(buf, typed)
	default:
		return writeCanonicalReflect(buf, value)
	}
}

func writeCanonicalReflect(buf *bytes.Buffer, value any) error {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		buf.WriteString("null")
		return nil
	}

	switch reflected.Kind() {
	case reflect.Slice, reflect.Array:
		buf.WriteByte('[')
		for i := range reflected.Len() {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, reflected.Index(i).Interface()); err != nil {
				return fmt.Errorf("encode list item %d: %w", i, err)
			}
		}
		buf.WriteByte(']')
		return nil
	case reflect.Map:
		if reflected.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("canonical JSON object keys must be strings")
		}
		keys := reflected.MapKeys()
		names := make([]string, 0, len(keys))
		for _, key := range keys {
			names = append(names, key.String())
		}
		sort.Strings(names)

		buf.WriteByte('{')
		for i, name := range names {
			if i > 0 {
				buf.WriteByte(',')
			}
			encoded, err := json.Marshal(name)
			if err != nil {
				return fmt.Errorf("encode object key %q: %w", name, err)
			}
			buf.Write(encoded)
			buf.WriteByte(':')
			item := reflected.MapIndex(reflect.ValueOf(name))
			if err := writeCanonicalJSON(buf, item.Interface()); err != nil {
				return fmt.Errorf("encode object field %s: %w", name, err)
			}
		}
		buf.WriteByte('}')
		return nil
	default:
		return fmt.Errorf("unsupported canonical JSON value %T", value)
	}
}

func writeCanonicalList(buf *bytes.Buffer, values []any) error {
	buf.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeCanonicalJSON(buf, value); err != nil {
			return fmt.Errorf("encode list item %d: %w", i, err)
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeCanonicalObject(buf *bytes.Buffer, values map[string]any) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		encoded, err := json.Marshal(key)
		if err != nil {
			return fmt.Errorf("encode object key %q: %w", key, err)
		}
		buf.Write(encoded)
		buf.WriteByte(':')
		if err := writeCanonicalJSON(buf, values[key]); err != nil {
			return fmt.Errorf("encode object field %s: %w", key, err)
		}
	}
	buf.WriteByte('}')
	return nil
}

func isCanonicalIntegerText(text string) bool {
	if text == "" || strings.ContainsAny(text, ".eE") {
		return false
	}
	if strings.HasPrefix(text, "-") {
		text = text[1:]
		if text == "" {
			return false
		}
	}
	if len(text) > 1 && text[0] == '0' {
		return false
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
