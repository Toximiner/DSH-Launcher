package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var permissionID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func permissionLabel(value string) string {
	switch value {
	case "read-only":
		return tr("Только чтение")
	case "workspace-write":
		return tr("Запись в рабочую область")
	case "danger-full-access":
		return tr("Полный доступ")
	case "auto":
		return tr("Авто")
	case "custom":
		return tr("Особые разрешения")
	}
	return singleLine(value)
}
func readPermissionSelection(ctx context.Context, api *dshAPI, id string) (string, error) {
	p, err := projectionValues(ctx, api, id)
	current := stringValue(objectValue(p["permissions"]), "currentValue")
	if err != nil || current == "" {
		return "", errors.New(tr("Права доступа недоступны в этой сессии."))
	}
	return current, nil
}
func readPermissions(ctx context.Context, api *dshAPI, id string) ([]benchRow, error) {
	current, err := readPermissionSelection(ctx, api, id)
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Options []struct {
			Value       string `json:"value"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"options"`
	}
	if err = api.rpc(ctx, "permissionPresets/catalog", map[string]any{}, &catalog); err != nil {
		return nil, err
	}
	if len(catalog.Options) > 256 {
		return nil, errors.New("permission catalog too large")
	}
	var rows []benchRow
	for _, option := range catalog.Options {
		if !permissionID.MatchString(option.Value) || option.Value == "custom" {
			continue
		}
		label := permissionLabel(option.Value)
		if label == option.Value && option.Name != "" {
			label = singleLine(option.Name)
		}
		mark := ""
		if option.Value == current {
			mark = tr(" · выбран")
		}
		body := clean(option.Description)
		if body == "" {
			body = label
		}
		body += "\n\n" + tr("Меняет доступ к файлам и правила подтверждения только для этой сессии.")
		rows = append(rows, benchRow{ID: option.Value, Title: label + mark, Body: body, Data: map[string]any{"baseline": current}})
	}
	return rows, nil
}
func permissionPreflight(ctx context.Context, api *dshAPI, id, baseline, line string) error {
	target := strings.TrimPrefix(line, "/permission ")
	if !permissionID.MatchString(target) || baseline == "" || line != "/permission "+target {
		return errors.New("invalid permission selection")
	}
	rows, err := readPermissions(ctx, api, id)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == target && stringValue(row.Data, "baseline") == baseline {
			return nil
		}
	}
	return errors.New("permission selection or catalog changed")
}
