package main

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type packageCancelResult struct {
	dialog *workbench
	serial int
	status string
	err    error
}

func (m *model) cancelPackageOperation() tea.Cmd {
	d := m.workbench
	if d == nil || !d.busy || !d.writing || d.packageRequestID == "" || d.cancelRequested {
		return nil
	}
	d.cancelRequested = true
	id, serial, login, ctx := d.packageRequestID, d.serial, m.settingsLogin, d.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		result := packageCancelResult{dialog: d, serial: serial}
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			var value struct{ Status string }
			err = api.rpc(ctx, "pluginManager/cancelInstall", map[string]any{"requestId": id}, &value)
			result.status = value.Status
		}
		result.err = err
		return result
	}
}
func (m *model) handlePackageCancel(result packageCancelResult) {
	d := m.workbench
	if d == nil || d != result.dialog || d.serial != result.serial || !d.busy {
		return
	}
	d.cancelRequested = true
	defer func() { d.cancelStatus = d.note }()
	if result.err != nil {
		d.note = tr("Отмена не подтверждена; ожидаем итог операции.")
		return
	}
	switch result.status {
	case "cancelled":
		d.note = tr("Установка отменена; ожидаем итог и восстановление состояния.")
	case "too-late":
		d.note = tr("Пакет уже применяется; отмена недоступна.")
	default:
		d.note = tr("Активная установка не найдена; ожидаем итог операции.")
	}
}
