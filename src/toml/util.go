package toml

import (
	"bytes"
	"go/ast"
	"reflect"
	"strings"
	"unicode"
)

// toCamelCase returns a copy of the string s with all Unicode letters mapped to their camel case.
// It will convert to upper case previous letter of '_' and first letter, and remove letter of '_'.
func toCamelCase(s string) string {
	if s == "" {
		return ""
	}
	result := make([]rune, 0, len(s))
	upper := false
	for _, r := range s {
		if r == '_' {
			upper = true
			continue
		}
		if upper {
			result = append(result, unicode.ToUpper(r))
			upper = false
			continue
		}
		result = append(result, r)
	}
	result[0] = unicode.ToUpper(result[0])
	return string(result)
}

// toSnakeCase returns a copy of the string s with all Unicode letters mapped to their snake case.
// It will insert letter of '_' at position of previous letter of uppercase and all
// letters convert to lower case.
// toSnakeCase does not insert '_' letter into a common initialism word like ID, URL and so on.
func toSnakeCase(input string) string {
	var output bytes.Buffer
	var runes = []rune(input)
	for i, char := range runes {
		if unicode.IsUpper(char) {
			// NOTE: If this is not the first character AND it is before or after a lowercase character
			if i > 0 && (unicode.IsLower(runes[i-1]) || (i != len(runes)-1 && unicode.IsLower(runes[i+1]))) {
				output.WriteRune('_')
			}
			output.WriteRune(unicode.ToLower(char))
		} else {
			output.WriteRune(char)
		}
	}

	return output.String()
}

const (
	fieldTagName = "toml"
)

func findField(rv reflect.Value, name string) (field reflect.Value, fieldName string, found bool) {
	switch rv.Kind() {
	case reflect.Struct:
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			ft := rt.Field(i)
			if !ast.IsExported(ft.Name) {
				continue
			}
			if col, _ := extractTag(ft.Tag.Get(fieldTagName)); col == name {
				return rv.Field(i), ft.Name, true
			}
		}
		for _, name := range []string{
			strings.Title(name),
			toCamelCase(name),
			strings.ToUpper(name),
		} {
			if field := rv.FieldByName(name); field.IsValid() {
				return field, name, true
			}
		}
	case reflect.Map:
		return reflect.New(rv.Type().Elem()).Elem(), name, true
	}
	return field, "", false
}

func extractTag(tag string) (col, rest string) {
	tags := strings.SplitN(tag, ",", 2)
	if len(tags) == 2 {
		return strings.TrimSpace(tags[0]), strings.TrimSpace(tags[1])
	}
	return strings.TrimSpace(tags[0]), ""
}

func tableName(prefix, name string) string {
	if prefix != "" {
		return prefix + string(tableSeparator) + name
	}
	return name
}
