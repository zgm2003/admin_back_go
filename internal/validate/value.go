package validate

import (
	"reflect"
	"strings"
)

func intValue(value reflect.Value) (int, bool) {
	value = dereference(value)
	if !value.IsValid() {
		return 0, false
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(value.Int()), true
	default:
		return 0, false
	}
}

func trimmedString(value reflect.Value) string {
	value = dereference(value)
	if !value.IsValid() || value.Kind() != reflect.String {
		return ""
	}
	return strings.TrimSpace(value.String())
}

func dereference(value reflect.Value) reflect.Value {
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}
