package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type formNode struct {
	Type  string         `json:"type"`
	Dict  map[string]int `json:"dict"`
	Inner int            `json:"inner"`
	List  []int          `json:"list"`
	Value any            `json:"value"`
	Meta  map[string]any `json:"meta"`
}
type formSchema struct {
	UID  int                 `json:"uid"`
	Refs map[string]formNode `json:"refs"`
}
type formRow struct {
	key   string
	path  []string
	node  formNode
	known bool
	value any
}
type settingsPathOp struct {
	Op    string   `json:"op"`
	Path  []string `json:"path"`
	Value any      `json:"value,omitempty"`
}

func decodeFormSchema(raw json.RawMessage) (*formSchema, error) {
	var schema formSchema
	if len(raw) > 1<<20 || json.Unmarshal(raw, &schema) != nil || len(schema.Refs) > 10000 {
		return nil, fmt.Errorf("%s", tr("Схема раздела недоступна или слишком велика."))
	}
	if _, ok := schema.Refs[strconv.Itoa(schema.UID)]; !ok {
		return nil, fmt.Errorf("%s", tr("Формат схемы раздела не поддержан."))
	}
	return &schema, nil
}
func (s *formSchema) node(id int) formNode { return s.Refs[strconv.Itoa(id)] }
func (s *formSchema) fields(node formNode, depth int) map[string]int {
	result := map[string]int{}
	if depth > 32 {
		return result
	}
	for key, id := range node.Dict {
		result[key] = id
	}
	if node.Type == "intersect" {
		for _, id := range node.List {
			for key, child := range s.fields(s.node(id), depth+1) {
				result[key] = child
			}
		}
	}
	return result
}
func valueAt(root any, path []string) (any, bool) {
	value := root
	for _, key := range path {
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[key]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(current) {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}
func editValue(root any, path []string, value any, unset bool) any {
	if len(path) == 0 {
		if unset {
			return nil
		}
		return value
	}
	key := path[0]
	if array, ok := root.([]any); ok {
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index > len(array) {
			return root
		}
		if len(path) == 1 && unset {
			if index < len(array) {
				return append(array[:index], array[index+1:]...)
			}
			return array
		}
		if index == len(array) {
			array = append(array, nil)
		}
		array[index] = editValue(array[index], path[1:], value, unset)
		return array
	}
	object, ok := root.(map[string]any)
	if !ok {
		object = map[string]any{}
	}
	if len(path) == 1 && unset {
		delete(object, key)
	} else {
		object[key] = editValue(object[key], path[1:], value, unset)
	}
	return object
}
func detached(value any) any {
	raw, _ := json.Marshal(value)
	var result any
	_ = json.Unmarshal(raw, &result)
	return result
}
func pathKey(path []string) string { raw, _ := json.Marshal(path); return string(raw) }
func pathUnder(path, parent []string) bool {
	if len(path) < len(parent) {
		return false
	}
	for i, key := range parent {
		if path[i] != key {
			return false
		}
	}
	return true
}
func (s *formSchema) at(path []string) (formNode, bool) {
	node := s.node(s.UID)
	for _, key := range path {
		switch node.Type {
		case "object", "intersect":
			id, ok := s.fields(node, 0)[key]
			if !ok {
				return formNode{}, false
			}
			node = s.node(id)
		case "dict", "array":
			node = s.node(node.Inner)
		default:
			return formNode{}, false
		}
	}
	return node, node.Type != ""
}
func (s *formSchema) rows(root any, path []string) []formRow {
	node, _ := s.at(path)
	value, _ := valueAt(root, path)
	keys := map[string]bool{}
	for key := range s.fields(node, 0) {
		keys[key] = true
	}
	switch v := value.(type) {
	case map[string]any:
		for key := range v {
			keys[key] = true
		}
	case []any:
		for i := range v {
			keys[strconv.Itoa(i)] = true
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if node.Type == "array" {
			a, _ := strconv.Atoi(ordered[i])
			b, _ := strconv.Atoi(ordered[j])
			return a < b
		}
		return ordered[i] < ordered[j]
	})
	var rows []formRow
	for _, key := range ordered {
		p := append(append([]string{}, path...), key)
		n, known := s.at(p)
		v, _ := valueAt(root, p)
		rows = append(rows, formRow{key, p, n, known, v})
	}
	return rows
}
func (s *formSchema) enums(node formNode) []any {
	if node.Type != "union" {
		return nil
	}
	var values []any
	for _, id := range node.List {
		child := s.node(id)
		if child.Type != "const" {
			return nil
		}
		values = append(values, child.Value)
	}
	return values
}
func nodeRole(node formNode) string { role, _ := node.Meta["role"].(string); return role }
func formContainer(node formNode) bool {
	return node.Type == "object" || node.Type == "intersect" || node.Type == "dict" || node.Type == "array"
}
func formDefault(node formNode) any {
	if value, ok := node.Meta["default"]; ok {
		return detached(value)
	}
	switch node.Type {
	case "object", "dict", "intersect":
		return map[string]any{}
	case "array":
		return []any{}
	case "boolean":
		return false
	case "number":
		return float64(0)
	case "const":
		return node.Value
	default:
		return ""
	}
}
func formSummary(row formRow) string {
	if nodeRole(row.node) == "secret" {
		return tr("[секрет скрыт]")
	}
	if formContainer(row.node) {
		return "[" + row.node.Type + "]"
	}
	if row.value == nil {
		return tr("[не задано]")
	}
	raw, _ := json.Marshal(row.value)
	return clean(strings.ReplaceAll(string(raw), "\n", " "))
}
