package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
)

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, dst any, required ...string) error {
	if r.Body == nil {
		return invalidJSON()
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return DecodeStrictJSON(r.Body, dst, required...)
}

// DecodeStrictJSON validates the exact object keys before letting encoding/json
// populate dst. A required key may be null if its endpoint permits null.
func DecodeStrictJSON(reader io.Reader, dst any, required ...string) error {
	target := reflect.ValueOf(dst)
	if !target.IsValid() || target.Kind() != reflect.Pointer || target.IsNil() {
		return invalidJSON()
	}
	dec := json.NewDecoder(reader)
	var raw json.RawMessage
	if dec.Decode(&raw) != nil || dec.Decode(new(any)) != io.EOF {
		return invalidJSON()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return invalidJSON()
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	if err := checkJSONKeys(scan, target.Type()); err != nil {
		return err
	}
	for _, field := range required {
		if _, ok := fields[field]; !ok {
			return InvalidJSONField(field)
		}
	}
	decoded := reflect.New(target.Elem().Type())
	if err := json.Unmarshal(raw, decoded.Interface()); err != nil {
		var mismatch *json.UnmarshalTypeError
		if errors.As(err, &mismatch) && mismatch.Field != "" {
			return InvalidJSONField(mismatch.Field)
		}
		return invalidJSON()
	}
	target.Elem().Set(decoded.Elem())
	return nil
}

func InvalidJSONField(field string) *Error {
	err := invalidJSON()
	err.Fields = map[string]string{field: "invalid"}
	return err
}

func invalidJSON() *Error {
	return &Error{Code: "VALIDATION_ERROR", HTTPStatus: http.StatusBadRequest}
}

var rawJSONType = reflect.TypeFor[json.RawMessage]()

func checkJSONKeys(dec *json.Decoder, target reflect.Type) error {
	for target != nil && target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	token, err := dec.Token()
	if err != nil {
		return invalidJSON()
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for dec.More() {
			token, err := dec.Token()
			key, ok := token.(string)
			if err != nil || !ok {
				return invalidJSON()
			}
			fieldType, allowed := jsonFieldType(target, key)
			if seen[key] || !allowed {
				return InvalidJSONField(key)
			}
			seen[key] = true
			if err := checkJSONKeys(dec, fieldType); err != nil {
				return err
			}
		}
	case json.Delim('['):
		var elem reflect.Type
		if target != nil && target != rawJSONType && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array) {
			elem = target.Elem()
		}
		for dec.More() {
			if err := checkJSONKeys(dec, elem); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	if _, err := dec.Token(); err != nil {
		return invalidJSON()
	}
	return nil
}

func jsonFieldType(target reflect.Type, key string) (reflect.Type, bool) {
	if target == nil || target.Kind() == reflect.Interface || target == rawJSONType {
		return nil, true
	}
	if target.Kind() == reflect.Map {
		return target.Elem(), true
	}
	if target.Kind() == reflect.Struct {
		for index := range target.NumField() {
			field := target.Field(index)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if field.IsExported() && name != "" && name != "-" && name == key {
				return field.Type, true
			}
		}
	}
	return nil, false
}
