package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, dst any) error {
	if r.Body == nil {
		return invalidJSON()
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return DecodeStrictJSON(r.Body, dst)
}

// DecodeStrictJSON checks exact JSON tags at every level before populating dst.
// Fields without omitempty are required. Only pointer fields accept null.
func DecodeStrictJSON(reader io.Reader, dst any) error {
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
	if err := checkJSONValue(scan, target.Elem().Type(), nil); err != nil {
		return err
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

func checkJSONValue(dec *json.Decoder, target reflect.Type, path []byte) error {
	token, err := dec.Token()
	if err != nil {
		return invalidJSON()
	}
	if token == nil && target != nil && target.Kind() != reflect.Pointer {
		return InvalidJSONField(string(path))
	}
	for target != nil && target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		fields := jsonObjectFields(target)
		for dec.More() {
			token, err := dec.Token()
			key, ok := token.(string)
			if err != nil || !ok {
				return invalidJSON()
			}
			fieldPath := jsonFieldPath(path, key)
			fieldType, allowed := jsonFieldType(target, fields, key)
			if seen[key] || !allowed {
				return InvalidJSONField(string(fieldPath))
			}
			seen[key] = true
			if err := checkJSONValue(dec, fieldType, fieldPath); err != nil {
				return err
			}
		}
		for _, field := range fields {
			if field.required && !seen[field.name] {
				return InvalidJSONField(string(jsonFieldPath(path, field.name)))
			}
		}
	case json.Delim('['):
		var elem reflect.Type
		if target != nil && target != rawJSONType && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array) {
			elem = target.Elem()
		}
		for index := 0; dec.More(); index++ {
			elemPath := append(strconv.AppendInt(append(path, '['), int64(index), 10), ']')
			if err := checkJSONValue(dec, elem, elemPath); err != nil {
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

type jsonObjectField struct {
	name     string
	typ      reflect.Type
	required bool
}

func jsonObjectFields(target reflect.Type) []jsonObjectField {
	if target == nil || target.Kind() != reflect.Struct {
		return nil
	}
	var fields []jsonObjectField
	for index := range target.NumField() {
		field := target.Field(index)
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if field.IsExported() && name != "" && name != "-" {
			fields = append(fields, jsonObjectField{name, field.Type, !slices.Contains(strings.Split(options, ","), "omitempty")})
		}
	}
	return fields
}

func jsonFieldType(target reflect.Type, fields []jsonObjectField, key string) (reflect.Type, bool) {
	if target == nil || target.Kind() == reflect.Interface || target == rawJSONType {
		return nil, true
	}
	if target.Kind() == reflect.Map {
		return target.Elem(), true
	}
	for _, field := range fields {
		if field.name == key {
			return field.typ, true
		}
	}
	return nil, false
}

func jsonFieldPath(parent []byte, key string) []byte {
	if len(parent) != 0 {
		parent = append(parent, '.')
	}
	return append(parent, key...)
}
