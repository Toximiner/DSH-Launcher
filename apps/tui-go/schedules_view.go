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
	refresh, add := "["+tr("Обновить")+"]", "[+ "+tr("Новая задача")+"]"
	toolbar := refresh + " " + add
	lines := []string{bar(heading().Render(tr("Автоматизация")), accent().Render(toolbar), w), muted().Render(tr("Задачи всего Host · время в часовом поясе терминала"))}
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
		text := fmt.Sprintf("[%s %d]", label, count)
		hits = append(hits, pluginHit{area{box.x + lipgloss.Width(tabs), box.y + 2, lipgloss.Width(text), 1}, "filter", i})
		if i == d.scheduleFilter {
			tabs += accent().Bold(true).Render(text) + " "
		} else {
			tabs += muted().Render(text) + " "
		}
	}
	d.search.Width = max(1, w-10)
	lines = append(lines, tabs, d.search.View(), muted().Render(strings.Repeat("─", w)))
	count := max(1, (box.height-8)/4)
	rows := d.visible()
	d.index = max(0, min(d.index, max(0, len(rows)-1)))
	start := max(0, min(d.index-count+1, max(0, len(rows)-count)))
	for i := 0; i < count; i++ {
		index := start + i
		if index >= len(rows) {
			text := ""
			if i == 0 {
				text = tr("Нет задач. Ctrl+N — создать; F2 — сменить фильтр.")
			}
			lines = append(lines, muted().Render(text), "", "", "")
			continue
		}
		row := rows[index]
		marker := "  "
		title := singleLine(stringValue(row.Data, "title"))
		if title == "" {
			title = row.Title
		}
		if index == d.index {
			marker = "› "
			title = accent().Bold(true).Render(title)
		}
		state := "[" + scheduleStatus(stringValue(row.Data, "status")) + "]"
		hits = append(hits, pluginHit{area{box.x, box.y + len(lines), w, 3}, "select", index}, pluginHit{area{box.x + w - lipgloss.Width(state), box.y + len(lines), lipgloss.Width(state), 1}, "toggle", index})
		label := tr("Следующий запуск: ")
		if stringValue(row.Data, "status") != "active" {
			label = tr("Таймер выключен · ")
		}
		lines = append(lines, bar(marker+title, accent().Render(state), w), muted().Render("  "+label+scheduleTime(stringValue(row.Data, "scheduledAt"))), "  "+singleLine(stringValue(row.Data, "prompt")), "")
	}
	for len(lines) < box.height-3 {
		lines = append(lines, "")
	}
	note := d.note
	if d.busy {
		note = tr("Запрос dsh…")
	}
	lines = append(lines, muted().Render(note), muted().Render(tr("↑/↓ выбор · Enter детали/история · Space пауза · F2 фильтр")), muted().Render(tr("Ctrl+N создать · F3 правка · Ctrl+G запуск · Ctrl+X удалить · Esc назад")))
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
