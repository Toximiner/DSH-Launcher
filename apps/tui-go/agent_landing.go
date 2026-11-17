package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme")

type agentContext struct {
	id        string
	epoch     int
	busy      bool
	catalog   *modelCatalog
	selection modelSelection
	presets   map[string]string
}
type agentContextResult struct {
	owner     *agentContext
	catalog   *modelCatalog
	selection modelSelection
	presets   map[string]string
}
type agentHit struct {
	area
	action string
}

func (m *model) loadAgentContext() tea.Cmd {
	if m.chat == nil || m.session == nil || m.session.ID != m.chat.id {
		return nil
	}
	if m.agentInfo == nil || m.agentInfo.id != m.session.ID || m.agentInfo.epoch != m.chat.epoch {
		m.agentInfo = &agentContext{id: m.session.ID, epoch: m.chat.epoch}
	}
	owner := m.agentInfo
	if owner.busy {
		return nil
	}
	owner.busy = true
	parent, login := m.chat.ctx, m.settingsLogin
	return func() tea.Msg {
		r := agentContextResult{owner: owner}
		ctx, cancel := context.WithTimeout(parent, 8*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err != nil {
			return r
		}
		defer api.close()
		r.catalog, r.selection, _ = readModelDirectory(ctx, api, owner.id)
		var presets struct {
			Presets []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"presets"`
		}
		if api.rpc(ctx, "agentPresets/list", map[string]any{}, &presets) == nil {
			r.presets = map[string]string{}
			for _, p := range presets.Presets {
				r.presets[p.ID] = singleLine(p.Name)
			}
		}
		return r
	}
}
func (m *model) handleAgentContext(r agentContextResult) {
	if r.owner == nil || r.owner != m.agentInfo || m.chat == nil || m.chat.id != r.owner.id || m.chat.epoch != r.owner.epoch {
		return
	}
	r.owner.busy = false
	if r.catalog != nil {
		r.owner.catalog, r.owner.selection = r.catalog, r.selection
	}
	if r.presets != nil {
		r.owner.presets = r.presets
	}
}
func singleLine(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(clean(s))
}

func (m *model) landingVisible() bool {
	if m.page != chatPage || len(m.lines) > 0 || m.sessionRequestVisible() || m.sessionError != "" {
		return false
	}
	if m.session == nil {
		return m.chat == nil
	}
	if m.session.Address.Kind == "subagent" || m.isSessionArchived(m.session.ID) || m.session.Running || m.session.Goal != nil && m.session.Goal.Goal.Phase == "active" {
		return false
	}
	s := m.chat
	return s != nil && s.id == m.session.ID && s.ready && !s.hasMore && s.clearedThrough < 0 && len(s.messages) == 0 && s.live == nil && s.note == "" && s.streamError == "" && m.prompts[s.id] == nil && len(m.visibleQueue()) == 0
}
func (m *model) landingStart() int { _, h := m.contentSize(); return max(1, (h-12)/3) }
func (m *model) composerBox() area {
	box := m.chatColumn()
	y := bodyTop + box.height - composerHeight - 1
	if m.landingVisible() {
		y = bodyTop + m.landingStart() + 4
	}
	return area{box.x, y, box.width, composerHeight}
}
func (m *model) modelButton() string {
	selected := modelSelection{}
	if m.agentInfo != nil {
		selected = m.agentInfo.selection
	}
	if m.session != nil && m.session.Model != nil {
		selected = *m.session.Model
	}
	if selected.Model == "" {
		return tr("Модель · F12")
	}
	name := singleLine(selected.Model)
	if m.agentInfo != nil && m.agentInfo.catalog != nil {
		for _, group := range m.agentInfo.catalog.Groups {
			if group.ID == selected.Provider {
				for _, model := range group.Models {
					if model.ID == selected.Model && model.Name != "" {
						name = singleLine(model.Name)
					}
				}
			}
		}
	}
	effort := tr("По умолчанию")
	if selected.Reasoning != "" {
		effort = singleLine(selected.Reasoning)
	}
	return name + " · " + effort
}
func (m *model) permissionButton() string {
	if m.session == nil || m.session.Permission == "" {
		return ""
	}
	return permissionLabel(m.session.Permission)
}
func (m *model) landingControls() (string, string, []agentHit) {
	box := m.composerBox()
	inner := box.width - 4
	hits := []agentHit{}
	button := func(label, action string, x, y, limit int) string {
		text := "[" + ansi.Truncate(singleLine(label), max(1, limit-2), "…") + "]"
		hits = append(hits, agentHit{area{x, y, lipgloss.Width(text), 1}, action})
		return accent().Render(text)
	}
	workspace := filepath.Base(m.workspace)
	for _, w := range m.side.workspaces {
		if m.session != nil {
			for _, id := range w.Sessions {
				if id == m.session.ID && w.Title != "" {
					workspace = w.Title
				}
			}
		}
	}
	preset := tr("Агент")
	if m.session != nil && m.session.AgentPreset != "" {
		preset = m.session.AgentPreset
		if m.agentInfo != nil && m.agentInfo.presets[preset] != "" {
			preset = m.agentInfo.presets[preset]
		}
	}
	half := (inner - 1) / 2
	folder := button("▸ "+workspace, "workspaces", box.x+2, box.y-1, half)
	agent := button(preset, "presets", box.x+2+lipgloss.Width(folder)+1, box.y-1, inner-lipgloss.Width(folder)-1)
	contextLine := folder + " " + agent
	toolY := box.y + 3
	plus := button("+", "compose", box.x+2, toolY, 3)
	left := plus
	permission := m.permissionButton()
	// Reserve room for the model at the minimum supported width.
	if permission != "" {
		left += " " + button(permission, "permissions", box.x+6, toolY, max(8, (inner-7)/2))
	}
	modelWidth := max(8, inner-lipgloss.Width(left)-1)
	modelLabel := "[" + ansi.Truncate(singleLine(m.modelButton()), modelWidth-2, "…") + "]"
	modelX := box.x + 2 + inner - lipgloss.Width(modelLabel)
	hits = append(hits, agentHit{area{modelX, toolY, lipgloss.Width(modelLabel), 1}, "models"})
	tools := bar(left, accent().Render(modelLabel), inner)
	return contextLine, tools, hits
}
func (m *model) landingView(title string, w, h int) string {
	start := m.landingStart()
	contextLine, tools, _ := m.landingControls()
	input := m.input
	input.Placeholder = tr("Опишите задачу или /help…")
	_, hint := m.composerText()
	if m.session != nil {
		hint = tr("Enter — отправить · Alt+E редактор")
	}
	border := theme.Accent
	if m.navFocused {
		border = theme.Border
	}
	composer := input.View() + "\n\n" + tools + "\n" + muted().Render(ansi.Truncate(hint, w-4, "…"))
	lines := make([]string, start)
	lines = append(lines, lipgloss.PlaceHorizontal(w, lipgloss.Center, heading().Render("◆ "+tr("С чего начнём?"))))
	lines = append(lines, lipgloss.PlaceHorizontal(w, lipgloss.Center, muted().Render(ansi.Truncate(title, w, "…"))), "")
	lines = append(lines, "  "+contextLine, panel(w, composerHeight, composer, border))
	modeHint := tr("Введите сообщение.") + " " + tr("F2 режим · Alt+A агент · Alt+P доступ · F12 модель")
	if m.session == nil {
		modeHint = tr("Нет подключения к dsh.")
	}
	if m.session != nil {
		lines = append(lines, muted().Render(ansi.Truncate(tr("Режим: ")+m.modeLabel()+" · F2", w, "…")))
	}
	lines = append(lines, muted().Render(ansi.Truncate(modeHint, w, "…")))
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(w).Height(h).Render(content)
}
func (m *model) agentControl(action string) tea.Cmd {
	switch action {
	case "chat":
		return m.showChat()
	case "history":
		return m.openHistory()
	case "rename":
		return m.openSessionRename(true)
	case "fork":
		return m.openSessionFork(true)
	case "archive":
		return m.openSessionArchive(true, false)
	case "conversation":
		return m.openDialog("conversation")
	case "models":
		return m.openModels()
	case "compose":
		return m.openCompose()
	default:
		return m.openWorkbench(action)
	}
}
func (m *model) agentKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Paste || !msg.Alt || m.page != chatPage || m.navFocused {
		return nil, false
	}
	action := map[string]string{"alt+w": "workspaces", "alt+a": "presets", "alt+p": "permissions", "alt+e": "compose"}[msg.String()]
	if action == "" {
		return nil, false
	}
	return m.agentControl(action), true
}
func (m *model) landingMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.landingVisible() {
		return nil, false
	}
	_, _, hits := m.landingControls()
	for _, hit := range hits {
		if hit.contains(msg.X, msg.Y) {
			m.navFocused, m.toolFocused = false, false
			m.input.Focus()
			return m.agentControl(hit.action), true
		}
	}
	box := m.composerBox()
	if box.contains(msg.X, msg.Y) {
		m.navFocused, m.toolFocused = false, false
		return m.input.Focus(), true
	}
	return nil, false
}

func readAgentProjection(values map[string]json.RawMessage, view *sessionView) {
	_ = json.Unmarshal(values["agentPreset"], &view.AgentPreset)
	var model struct {
		Next *modelSelection `json:"next"`
	}
	if json.Unmarshal(values["modelSelection"], &model) == nil {
		view.Model = model.Next
	}
	var permission struct {
		Current string `json:"currentValue"`
	}
	if json.Unmarshal(values["permissions"], &permission) == nil {
		view.Permission = permission.Current
	}
}
