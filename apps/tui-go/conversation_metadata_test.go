package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestMessageMetadataOrdinaryUserHasNoClockOrAction(t *testing.T) {
	original := activeEnglish.Load()
	defer activeEnglish.Store(original)
	for _, english := range []bool{false, true} {
		activeEnglish.Store(english)
		for _, role := range []string{"", "ВЫ", englishCatalog["ВЫ"]} {
			message := chatMessage{human: true, role: role, text: "hello", time: 1700000000000, elapsedMs: 1500}
			if meta := messageMetadata(message, 75); meta != "" {
				t.Fatalf("ordinary user has permanent metadata: %q", meta)
			}
		}
	}
}

func TestMessageMetadataAssistantUsesOnlyHistoricalFacts(t *testing.T) {
	original := activeEnglish.Load()
	defer activeEnglish.Store(original)
	activeEnglish.Store(false)
	message := chatMessage{assistant: true, role: "АГЕНТ", text: "answer", time: 1700000000000, elapsedMs: 1500}
	meta := messageMetadata(message, 95)
	if meta != "Агент · 1.5 с" {
		t.Fatalf("unexpected assistant metadata: %q", meta)
	}
	if strings.Contains(meta, "Копировать") || strings.Contains(meta, ":") {
		t.Fatal("assistant metadata contains action or wall clock")
	}
	message.elapsedMs = 0
	if meta := messageMetadata(message, 95); meta != "Агент" {
		t.Fatalf("unknown elapsed duration was fabricated: %q", meta)
	}
	message.elapsedMs = -1
	if meta := messageMetadata(message, 95); meta != "Агент" {
		t.Fatalf("invalid elapsed duration displayed: %q", meta)
	}
	message.elapsedMs = 1 << 54
	if meta := messageMetadata(message, 95); meta != "Агент" {
		t.Fatalf("invalid large duration displayed: %q", meta)
	}
}

func TestMessageMetadataAssistantDurationUsesCurrentLocale(t *testing.T) {
	original := activeEnglish.Load()
	defer activeEnglish.Store(original)
	activeEnglish.Store(true)
	message := chatMessage{assistant: true, role: "АГЕНТ", elapsedMs: 1500}
	if meta := messageMetadata(message, 95); meta != "Agent · 1.5 s" {
		t.Fatalf("assistant duration retained old locale: %q", meta)
	}
}

func TestMessageMetadataPreservesSemanticIdentityAndLocale(t *testing.T) {
	original := activeEnglish.Load()
	defer activeEnglish.Store(original)
	sources := []string{"ВЫ · ОТВЕТЫ НА ВОПРОС", "АГЕНТ · НЕЗАВЕРШЁННЫЙ ОТВЕТ", "СООБЩЕНИЕ DSH", "РАЗРЕШЕНИЕ", "РАЗРЕШЕНИЕ · ОЖИДАЕТ ОТВЕТА"}
	for _, english := range []bool{false, true} {
		activeEnglish.Store(english)
		for _, source := range sources {
			for _, storedRole := range []string{source, englishCatalog[source]} {
				message := chatMessage{role: storedRole, text: "semantic message", human: source == "ВЫ · ОТВЕТЫ НА ВОПРОС", time: 1700000000000}
				if meta := messageMetadata(message, 95); meta != tr(source) {
					t.Fatalf("role %q lost or changed on locale switch: %q", source, meta)
				}
			}
		}
	}
}

func TestMessageMetadataFitsTerminalColumns(t *testing.T) {
	original := activeEnglish.Load()
	defer activeEnglish.Store(original)
	for _, english := range []bool{false, true} {
		activeEnglish.Store(english)
		for _, width := range []int{0, 1, 8, 75, 95, 96} {
			for _, message := range []chatMessage{{assistant: true, elapsedMs: 1500}, {role: strings.Repeat("世界🙂 arbitrary role ", 20)}, {role: "ВЫ · ОТВЕТЫ НА ВОПРОС", human: true}} {
				meta := messageMetadata(message, width)
				if strings.Contains(meta, "\n") || lipgloss.Width(meta) > width {
					t.Fatalf("metadata overflowed width %d: %q", width, meta)
				}
			}
		}
	}
}
