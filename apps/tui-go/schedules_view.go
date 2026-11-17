package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var scheduleStatuses = []string{"", "active", "paused", "ended"}

func (d *workbench) pageNumber() int {
	if d.kind == "schedules" {
		return schedulesPage
	}
	return pluginsPage
}
func scheduleStatus(status string) string {
	switch status {
	case "active":
		return tr("Включена")
	case "paused":
		return tr("На паузе")
	case "ended":
		return tr("Завершена")
	}
	return clean(status)
}
func scheduleTime(value string) string {
	if instant, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return instant.Local().Format("2006-01-02 15:04:05 MST")
	}
	if value == "" {
		return "—"
	}
	return clean(value)
}
func scheduleRule(data map[string]any) string {
	switch stringValue(data, "kind") {
	case "after", "at":
		return tr("Однократно")
	case "every":
		return fmt.Sprintf(tr("Каждые %v с"), data["everySeconds"])
	case "daily":
		return tr("Ежедневно · ") + stringValue(data, "time") + " · " + stringValue(data, "timeZone")
	case "weekly":
		return fmt.Sprintf(tr("Еженедельно · дни %v · "), data["weekdays"]) + stringValue(data, "time") + " · " + stringValue(data, "timeZone")
	case "cron":
		return stringValue(data, "expression") + " · " + stringValue(data, "timeZone")
	case "rrule":
		return stringValue(data, "rrule")
	}
	return tr("Правило времени недоступно")
}
func scheduleDetails(data map[string]any) string {
	target := objectValue(data["target"])
	destination := stringValue(target, "sessionId")
	if destination == "" {
		destination = stringValue(target, "kind")
	}
	label := tr("Следующий запуск: ")
	if stringValue(data, "status") != "active" {
		label = tr("Время по правилу (таймер выключен): ")
	}
	body := scheduleStatus(stringValue(data, "status")) + "\n" + scheduleRule(data) + "\n" + label + scheduleTime(stringValue(data, "scheduledAt")) + "\n" + tr("Сессия: ") + destination + "\n\n" + stringValue(data, "prompt")
	if data["executionActive"] == true {
		body += "\n\n" + tr("Задание в очереди или выполняется в сессии.")
	}
	return clean(body)
}
func scheduleHistory(value map[string]any) string {
	lines := []string{tr("История расписания · последние 100 записей"), ""}
	records := arrayValue(value["records"])
	if len(records) == 0 {
		lines = append(lines, tr("Пока нет записей доставки."))
	}
	for _, item := range records {
		record := objectValue(item)
		lines = append(lines, tr("Доставлено: ")+scheduleTime(stringValue(record, "deliveredAt")), tr("По расписанию: ")+scheduleTime(stringValue(record, "scheduledAt")), tr("Сессия: ")+stringValue(record, "targetSessionId"))
		if prompt := stringValue(record, "prompt"); prompt != "" {
			lines = append(lines, prompt)
		}
		lines = append(lines, "")
	}
	if value["earlierRecordsPruned"] == true || value["earlierRecordsUnavailable"] == true {
		lines = append(lines, tr("Часть ранней истории недоступна."))
	}
	if stringValue(value, "nextBefore") != "" {
		lines = append(lines, tr("Показаны последние 100 доставок; более ранние записи остаются в dsh."))
	}
	retention := objectValue(value["retention"])
	if retention != nil {
		lines = append(lines, fmt.Sprintf(tr("Хранение: до %v записей за %v дней."), retention["records"], retention["days"]))
	}
	return clean(strings.Join(lines, "\n"))
}
func (m *model) schedulesLayout(box area) ([]string, []pluginHit) {
	d := m.workbench
	w := box.width
	refresh, add := tr("Обновить")+" F5", "+ "+tr("Новая задача")+" Ctrl+N"
	toolbar := refresh + " · " + add
	lines := []string{bar(heading().Render(tr("Автоматизация")), muted().Render(toolbar), w), muted().Render(tr("Задачи всего Host · время в часовом поясе терминала"))}
	hits := []pluginHit{{area{box.x + w - lipgloss.Width(toolbar), box.y, lipgloss.Width(refresh), 1}, "refresh", 0}, {area{box.x + w - lipgloss.Width(add), box.y, lipgloss.Width(add), 1}, "create", 0}}
	tabs := ""
	for i, status := range scheduleStatuses {
		label := scheduleStatus(status)
		if i == 0 {
			label = tr("Все")
		}
		count := 0
		for _, row := range d.rows {
			if i == 0 || stringValue(row.Data, "status") == status {
				count++
			}
		}
		text := fmt.Sprintf("%s %d", label, count)
		hits = append(hits, pluginHit{area{box.x + lipgloss.Width(tabs), box.y + 2, lipgloss.Width(text), 1}, "filter", i})
		if i == d.scheduleFilter {
			tabs += heading().Render(text) + "  "
		} else {
			tabs += muted().Render(text) + "  "
		}
	}
	d.search.Width = max(1, w-10)
	lines = append(lines, tabs, d.search.View(), "", chooserGroup(tr("Расписания · весь Host")))
	rows := d.visible()
	d.index = max(0, min(d.index, max(0, len(rows)-1)))
	count := max(1, box.height-13)
	start, end := chooserWindow(d.index, len(rows), count)
	for index := start; index < end; index++ {
		row := rows[index]
		title := singleLine(stringValue(row.Data, "title"))
		if title == "" {
			title = row.Title
		}
		state := scheduleStatus(stringValue(row.Data, "status"))
		if row.Data["executionActive"] == true {
			state += " · " + tr("Задание в очереди или выполняется в сессии.")
		}
		// Use the exact same bounded hint as chooserItem: its visible state is the
		// toggle target, while the remainder of this one-line row only selects.
		state = ansi.Truncate(clean(state), max(1, w/2), "…")
		y := box.y + len(lines)
		hits = append(hits, pluginHit{area{box.x, y, w, 1}, "select", index}, pluginHit{area{box.x + w - lipgloss.Width(state), y, lipgloss.Width(state), 1}, "toggle", index})
		lines = append(lines, chooserItem(title, state, index == d.index, true, w))
	}
	if len(rows) == 0 {
		lines = append(lines, muted().Render(tr("Нет задач. Ctrl+N — создать; F2 — сменить фильтр.")))
	}
	for len(lines) < 6+count {
		lines = append(lines, "")
	}
	lines = append(lines, "")
	detail := []string{"", "", ""}
	if row, ok := d.selected(); ok {
		detail[0] = chooserGroup(scheduleRule(row.Data))
		label := tr("Следующий запуск: ")
		if stringValue(row.Data, "status") != "active" {
			label = tr("Таймер выключен · ")
		}
		detail[1] = muted().Render(label + scheduleTime(stringValue(row.Data, "scheduledAt")))
		detail[2] = muted().Render(singleLine(stringValue(row.Data, "prompt")))
	}
	lines = append(lines, detail...)
	note := d.note
	if d.busy {
		note = tr("Запрос dsh…")
	}
	lines = append(lines, muted().Render(singleLine(note)), muted().Render(tr("↑/↓ выбор · Enter детали/история · Space пауза · F2 фильтр")), muted().Render(tr("Ctrl+N создать · F3 правка · Ctrl+G запуск · Ctrl+X удалить · Esc назад")))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, w, "…")
	}
	return lines, hits
}
func (m *model) schedulesContent(box area) string {
	d := m.workbench
	if d.document != "" {
		title := d.document
		body := ""
		// Keep the selected snapshot: history does not replace the task's revision.
		for _, row := range d.rows {
			if row.ID == d.document {
				title = stringValue(row.Data, "title")
				body = row.Body + "\n\n"
				break
			}
		}
		d.view.Width = box.width
		d.view.Height = max(1, box.height-4)
		d.view.SetContent(ansi.Hardwrap(clean(body+d.fileBody), box.width, true))
		lines := heading().Render(ansi.Truncate(clean(title), box.width, "…")) + "\n" + muted().Render(tr("Правило и записи доставки")) + "\n" + d.view.View() + "\n" + muted().Render(ansi.Truncate(clean(d.note), box.width, "…")) + "\n" + muted().Render(tr("↑/↓ прокрутка · F5 обновить · Esc к списку"))
		return lipgloss.NewStyle().Width(box.width).Height(box.height).Render(lines)
	}
	lines, _ := m.schedulesLayout(box)
	return lipgloss.NewStyle().Width(box.width).Height(box.height).Render(strings.Join(lines, "\n"))
}
func (m *model) schedulesMouse(msg tea.MouseMsg) tea.Cmd {
	box := m.pluginsPageBox()
	if !box.contains(msg.X, msg.Y) {
		return nil
	}
	m.navFocused = false
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		key := tea.KeyDown
		if msg.Button == tea.MouseButtonWheelUp {
			key = tea.KeyUp
		}
		cmd, _ := m.benchKey(tea.KeyMsg{Type: key})
		return cmd
	}
	if msg.Button != tea.MouseButtonLeft || m.workbench.document != "" {
		return nil
	}
	_, hits := m.schedulesLayout(box)
	// Switch hit regions take precedence over their containing card.
	for i := len(hits) - 1; i >= 0; i-- {
		hit := hits[i]
		if !hit.contains(msg.X, msg.Y) {
			continue
		}
		switch hit.action {
		case "refresh":
			return m.loadWorkbench()
		case "create":
			return m.benchPrepare(tea.KeyCtrlN)
		case "filter":
			m.workbench.scheduleFilter = hit.index
			m.workbench.index = 0
		case "select":
			m.workbench.index = hit.index
		case "toggle":
			m.workbench.index = hit.index
			return m.benchPrepare(tea.KeySpace)
		}
		return nil
	}
	return nil
}
