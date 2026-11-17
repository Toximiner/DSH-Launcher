package main

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme")

// The native controller owns processes and disk transactions. Only immutable
// snapshots cross into the Bubble Tea model, including while a dialog is closed.
type runtimeSnapshot struct {
	Status, Detail, Log, Note, Login string
	Busy, Owned, Exited, Bell        bool
	Revision                         int
}
type runtimeControl interface {
	Snapshot() runtimeSnapshot
	Execute(context.Context, string, string) error
	Close()
}
type runtimeTick struct{}
type runtimeDone struct{ err error }
type runtimeDialog struct {
	note                      string
	kind                      string
	index                     int
	confirm, editing, focused bool
	input                     textinput.Model
	view                      viewport.Model
}
type runtimeAction struct {
	id, label, hint string
	input, confirm  bool
}

var runtimeActions = []runtimeAction{
	{"probe", "Проверить Node.js и dsh", "Проверка установленных программ и управляемой версии.", false, false},
	{"install", "Установить dsh…", "Версия npm: latest или точная версия. Установка в каталог TUI без sudo.", true, true},
	{"start", "Запустить свой dsh", "Создать процесс с указанными ниже параметрами и подключиться.", false, true},
	{"stop", "Остановить свой dsh", "Прервёт все задачи этого процесса. Чужой процесс не затрагивается.", false, true},
	{"restart", "Перезапустить свой dsh", "Прервёт задачи; повторно подключит выбранную сессию без отправки сообщений.", false, true},
	{"bin", "Путь к dsh…", "dsh из PATH либо абсолютный путь. Управляемая установка имеет приоритет.", true, true},
	{"profile", "Профиль dsh…", "Общий с GUI профиль; настройка запуска хранится только в TUI.", true, true},
	{"port", "Порт dsh…", "Локальный порт от 1024 до 65535.", true, true},
	{"cwd", "Рабочая папка…", "Абсолютный путь к существующей папке.", true, true},
}
var updateActions = []runtimeAction{
	{"check", "Проверить версии и изменения", "npm latest для dsh; релизы GitHub с бинарником для TUI.", false, false},
	{"update-dsh", "Обновить dsh…", "latest или точная версия. Сначала остановите свой dsh. Старая версия сохраняется.", true, true},
	{"rollback-dsh", "Вернуть предыдущий dsh", "Переключение на сохранённую рабочую установку. Профиль не откатывается.", false, true},
	{"update-tui", "Обновить TUI из релиза", "Проверенный SHA-256 релиза; старая версия сохраняется. Затем нужен перезапуск.", false, true},
	{"local-tui", "Установить TUI из файла…", "Абсолютный путь и SHA-256 через пробел. Только готовый Linux-бинарник.", true, true},
	{"rollback-tui", "Вернуть предыдущий TUI", "Восстановить резервный бинарник; затем перезапустить клиент.", false, true},
}
var diagnosticActions = []runtimeAction{
	{"logs", "Журнал dsh и операций", "Хвост вывода только своего процесса. PgUp/PgDn — прокрутка.", false, false},
	{"report", "Сохранить диагностический отчёт…", "Папка назначения (пусто — каталог отчётов TUI). Без переписки и настроек dsh.", true, true},
	{"bell", "Звук терминальных уведомлений", "Включить/выключить BEL. Состояние и сообщения видны и без звука.", false, false},
}

func (d *runtimeDialog) actions() []runtimeAction {
	switch d.kind {
	case "updates":
		return updateActions
	case "diagnostics":
		return diagnosticActions
	}
	return runtimeActions
}
func runtimePoll() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return runtimeTick{} })
}
func (m *model) openRuntime(kind string) tea.Cmd {
	if m.runtime == nil {
		m.append(tr("Управление установкой доступно в нативном Linux-клиенте."))
		return nil
	}
	d := &runtimeDialog{kind: kind, focused: m.input.Focused(), input: textinput.New(), view: viewport.New(1, 1)}
	d.input.CharLimit = 4096
	d.input.Prompt = "> "
	m.runtimeDialog = d
	m.input.Blur()
	return nil
}
func (m *model) runtimeKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.runtimeDialog
	if d == nil {
		return nil, false
	}
	if msg.Type == tea.KeyRunes && (msg.Paste || len(msg.Runes) > 1) {
		if d.editing {
			var cmd tea.Cmd
			d.input, cmd = d.input.Update(msg)
			return cmd, true
		}
		return nil, true
	}
	k := msg.String()
	if k == "ctrl+c" || k == "ctrl+d" {
		return tea.Quit, true
	}
	if k == "esc" {
		if d.confirm || d.editing {
			d.confirm = false
			d.editing = false
			d.input.Blur()
			return nil, true
		}
		m.runtimeDialog = nil
		if d.focused {
			return m.input.Focus(), true
		}
		return nil, true
	}
	if d.confirm {
		if k == "enter" {
			d.confirm = false
			d.editing = false
			d.input.Blur()
			a := d.actions()[d.index]
			return m.runRuntime(a.id, d.input.Value()), true
		}
		return nil, true
	}
	if d.editing {
		if k == "enter" {
			d.editing = false
			d.input.Blur()
			d.confirm = d.actions()[d.index].confirm
			return nil, true
		}
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		return cmd, true
	}
	if msg.Type == tea.KeyRunes && (msg.Paste || len(msg.Runes) > 1) {
		return nil, true
	}
	switch k {
	case "up":
		d.index = (d.index + len(d.actions()) - 1) % len(d.actions())
	case "down", "tab":
		d.index = (d.index + 1) % len(d.actions())
	case "pgup":
		d.view.HalfPageUp()
	case "pgdown":
		d.view.HalfPageDown()
	case "enter":
		if m.runtimeState.Busy {
			return nil, true
		}
		a := d.actions()[d.index]
		d.input.SetValue("")
		if a.input {
			if a.id == "install" || a.id == "update-dsh" {
				d.input.SetValue("latest")
			}
			d.editing = true
			return d.input.Focus(), true
		}
		if a.confirm {
			d.confirm = true
			return nil, true
		}
		return m.runRuntime(a.id, ""), true
	}
	return nil, true
}
func (m *model) runRuntime(action, value string) tea.Cmd {
	if m.runtimeState.Busy {
		return nil
	}
	// Never acquire process ownership merely by attaching to a login URL.
	if (action == "start" || action == "restart") && m.settingsLogin != "" && !m.runtimeState.Owned {
		m.runtimeDialog.note = tr("Подключён внешний dsh. Откройте отдельный клиент без --url для своего процесса.")
		return nil
	}
	if action == "stop" || action == "restart" {
		if !m.runtimeState.Owned {
			m.runtimeDialog.note = tr("Нет своего работающего процесса.")
			return nil
		}
		m.stopChat()
		m.chat = nil
		m.settingsLogin = ""
	}
	if m.runtimeDialog != nil {
		m.runtimeDialog.note = ""
	}

	control := m.runtime
	ctx := m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	m.runtimeState.Busy = true
	return func() tea.Msg { return runtimeDone{control.Execute(ctx, action, value)} }
}
func (m *model) handleRuntimeTick() tea.Cmd {
	if m.runtime == nil {
		return nil
	}
	old := m.runtimeState
	m.runtimeState = m.runtime.Snapshot()
	if old.Revision != m.runtimeState.Revision {
		m.events = append(m.events, clean(m.runtimeState.Status))
		if len(m.events) > 200 {
			m.events = m.events[len(m.events)-200:]
		}
		m.rebuild()
	}
	var connect tea.Cmd
	if m.runtimeState.Login != "" && m.runtimeState.Login != m.settingsLogin {
		id := ""
		if m.session != nil {
			id = m.session.ID
		}
		m.stopChat()
		m.chat = nil
		m.session = nil
		m.settingsLogin = m.runtimeState.Login
		if id != "" {
			connect = m.sessionOperation("connect", id)
		} else {
			connect = m.sessionOperation("new", "")
		}
	}
	if m.runtimeState.Exited && (old.Owned || m.settingsLogin != "") {
		m.stopChat()
		m.chat = nil
		m.settingsLogin = ""
		m.append(tr("Процесс dsh завершился. /runtime — журнал и повторный запуск."))
	}
	return tea.Batch(runtimePoll(), connect)
}
func (m *model) runtimeView(base string) string {
	d := m.runtimeDialog
	if d == nil {
		return base
	}
	width, height := min(96, m.width-3), m.height-4
	w := width - 4
	title := map[string]string{"runtime": tr("Установка и процесс dsh"), "updates": tr("Обновления и откат"), "diagnostics": tr("Диагностика и уведомления")}[d.kind]
	trim := func(s string) string { return ansi.Truncate(clean(s), w, "…") }
	lines := []string{heading().Render(title), trim(m.runtimeState.Status)}
	actions := d.actions()
	listH := min(len(actions), max(3, (height-10)/2))
	start := max(0, min(d.index-listH/2, len(actions)-listH))
	for i := start; i < start+listH; i++ {
		lines = append(lines, selectedRow(tr(actions[i].label), d.index == i, w))
	}
	body := tr(actions[d.index].hint) + "\n" + m.runtimeState.Detail
	if d.kind == "updates" {
		body = tr(actions[d.index].hint) + "\n" + m.runtimeState.Log + "\n" + m.runtimeState.Detail
	}
	if m.runtimeState.Busy {
		body = m.runtimeState.Log
	}
	if d.kind == "diagnostics" && d.index <= 1 {
		body = m.runtimeState.Log
	}
	d.view.Width = w
	d.view.Height = max(1, height-len(lines)-7)
	d.view.SetContent(ansi.Hardwrap(clean(body), w, true))
	if m.runtimeState.Busy {
		d.view.GotoBottom()
	}
	lines = append(lines, strings.Split(d.view.View(), "\n")...)
	state := m.runtimeState.Note
	if d.note != "" {
		state = d.note
	}
	if m.runtimeState.Busy {
		state = tr("Выполняется… Ctrl+D — выйти с отменой операции.")
	}
	if d.editing {
		d.input.Width = w - 3
		lines = append(lines, d.input.View())
	} else {
		lines = append(lines, trim(state))
	}
	hint := tr("↑/↓ выбор · Enter действие · PgUp/PgDn детали · Esc назад")
	if d.confirm {
		hint = tr("Подтвердить: Enter · Отменить: Esc")
		lines[len(lines)-1] = trim(tr("Подтвердить: ") + tr(actions[d.index].label) + " " + d.input.Value())
	}
	if d.editing {
		hint = tr("Введите значение · Enter далее · Esc отмена")
	}
	lines = append(lines, trim(hint))
	// Fill the box deterministically at the minimum terminal size.
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	lines = lines[:height-2]
	return overlay(base, panel(width, height, strings.Join(lines, "\n"), theme.Highlight), (m.width-1-width)/2, 2, m.width-1)
}

func (m *model) runtimeNotice(message string) {
	m.events = append(m.events, message)
	if len(m.events) > 200 {
		m.events = m.events[len(m.events)-200:]
	}
	if c, ok := m.runtime.(interface{ Notice(string) }); ok {
		c.Notice(message)
	}
}
