package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"unicode/utf16"
)

// This checks portable schema constraints only. Host-side refinements, patterns,
// transforms and unsupported types remain the authoritative backend's job.
func (s *formSchema) validate(node formNode, value any) error {
	budget := 10000
	return s.validateNode(node, value, 0, &budget)
}
func (s *formSchema) validateNode(node formNode, value any, depth int, budget *int) error {
	*budget--
	if depth > 32 || *budget < 0 {
		return errors.New(tr("Значение слишком сложное для локальной проверки."))
	}
	if value == nil {
		if required, _ := node.Meta["required"].(bool); required {
			return errors.New(tr("Обязательное значение не задано."))
		}
		return nil
	}
	var measure float64
	rangeCheck := false
	switch node.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return errors.New(tr("Ожидается строка."))
		}
		if nodeRole(node) == "credential-ref" && text != "" && !credentialRefPattern.MatchString(text) {
			return errors.New(tr("Некорректное имя ссылки на ключ."))
		}
		measure, rangeCheck = float64(len(utf16.Encode([]rune(text)))), true
	case "number":
		number, ok := value.(float64)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return errors.New(tr("Ожидается конечное число."))
		}
		measure, rangeCheck = number, true
		if step, ok := node.Meta["step"].(float64); ok && step != 0 {
			minimum, _ := node.Meta["min"].(float64)
			multiple := (number - minimum) / math.Abs(step)
			if math.Abs(multiple-math.Round(multiple)) > 1e-9 {
				return fmt.Errorf(tr("Число не соответствует шагу %g от %g."), step, minimum)
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return errors.New(tr("Ожидается true или false."))
		}
	case "const":
		a, _ := json.Marshal(value)
		b, _ := json.Marshal(node.Value)
		if string(a) != string(b) {
			return errors.New(tr("Значение не входит в варианты схемы."))
		}
	case "array", "tuple":
		array, ok := value.([]any)
		if !ok {
			return errors.New(tr("Ожидается массив JSON."))
		}
		measure, rangeCheck = float64(len(array)), node.Type == "array"
		for i, child := range array {
			inner := s.node(node.Inner)
			if node.Type == "tuple" {
				if i >= len(node.List) {
					break
				}
				inner = s.node(node.List[i])
			}
			if err := s.validateNode(inner, child, depth+1, budget); err != nil {
				return err
			}
		}
		if node.Type == "tuple" {
			for _, id := range node.List[min(len(array), len(node.List)):] {
				if err := s.validateNode(s.node(id), nil, depth+1, budget); err != nil {
					return err
				}
			}
		}
	case "object", "dict":
		object, ok := value.(map[string]any)
		if !ok {
			return errors.New(tr("Ожидается объект JSON."))
		}
		if node.Type == "object" {
			for key, id := range node.Dict {
				if err := s.validateNode(s.node(id), object[key], depth+1, budget); err != nil {
					return err
				}
			}
		} else {
			for _, child := range object {
				if err := s.validateNode(s.node(node.Inner), child, depth+1, budget); err != nil {
					return err
				}
			}
		}
	case "union":
		for _, id := range node.List {
			if s.validateNode(s.node(id), value, depth+1, budget) == nil {
				return nil
			}
		}
		return errors.New(tr("Значение не соответствует ни одному варианту схемы."))
	case "intersect":
		for _, id := range node.List {
			if err := s.validateNode(s.node(id), value, depth+1, budget); err != nil {
				return err
			}
		}
	}
	if rangeCheck {
		minimum, hasMinimum := node.Meta["min"].(float64)
		maximum, hasMaximum := node.Meta["max"].(float64)
		if node.Type == "array" && s.node(node.Inner).Meta["default"] != nil {
			hasMinimum = false
		}
		if hasMinimum && measure < minimum {
			return fmt.Errorf(tr("Минимум по схеме: %g."), minimum)
		}
		if hasMaximum && measure > maximum {
			return fmt.Errorf(tr("Максимум по схеме: %g."), maximum)
		}
	}
	return nil
}
