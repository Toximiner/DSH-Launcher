package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func documentPreviewKind(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return "image"
	case ".pdf":
		return "pdf"
	case ".html", ".htm":
		return "html"
	}
	return ""
}

func readDocumentPreview(ctx context.Context, api *dshAPI, session, name string) (string, error) {
	var file struct {
		Data    []byte
		Version string
		EOF     bool
		Offset  int
	}
	if err := api.rpc(ctx, "workspaceFiles/readBytes", map[string]any{"workspaceFileScopeId": session, "path": name, "options": map[string]any{}}, &file); err != nil {
		return "", err
	}
	if !file.EOF || file.Offset != 0 || len(file.Data) > 6<<20 {
		return "", errors.New(tr("Для просмотра нужен полный файл до 6 МиБ."))
	}
	var body string
	var err error
	switch documentPreviewKind(name) {
	case "image":
		body, err = imagePreview(file.Data)
	case "pdf":
		body, err = pdfPreview(ctx, file.Data)
	case "html":
		body, err = htmlPreview(file.Data)
	default:
		return "", errors.New(tr("Неподдерживаемый формат документа."))
	}
	return fmt.Sprintf(tr("%s · версия %s\n\n%s"), clean(name), clean(file.Version), body), err
}

// A small half-block thumbnail works without terminal-specific image protocols.
func imagePreview(data []byte) (string, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 20_000_000 {
		return "", errors.New(tr("Изображение повреждено или превышает 20 млн пикселей."))
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", errors.New(tr("Не удалось прочитать изображение."))
	}
	width := min(48, config.Width)
	height := max(1, min(24, config.Height*width/config.Width))
	var b strings.Builder
	fmt.Fprintf(&b, tr("%s · %d × %d · миниатюра\n"), format, config.Width, config.Height)
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x++ {
			colorAt := func(row int) lipgloss.Color {
				r, g, bl, a := img.At(img.Bounds().Min.X+x*config.Width/width, img.Bounds().Min.Y+min(row, height-1)*config.Height/height).RGBA()
				// Composite alpha over the dark preview background.
				c := func(v uint32) uint32 { return (v + (0xffff-a)*0x18/255) >> 8 }
				return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c(r), c(g), c(bl)))
			}
			b.WriteString(lipgloss.NewStyle().Foreground(colorAt(y)).Background(colorAt(y + 1)).Render("▀"))
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

type previewOutput struct{ bytes.Buffer }

func (w *previewOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > 256<<10 {
		return 0, errors.New("document output limit")
	}
	return w.Buffer.Write(p)
}
func pdfPreview(ctx context.Context, data []byte) (string, error) {
	executable, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", errors.New(tr("Для PDF установите poppler-utils (pdftotext)."))
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-layout", "-", "-")
	command.Stdin = bytes.NewReader(data)
	out := &previewOutput{}
	command.Stdout = out
	if command.Run() != nil {
		return "", errors.New(tr("PDF не прочитан: повреждение, защита или лимит 256 КиБ/10 с."))
	}
	body := strings.TrimSpace(clean(out.String()))
	if body == "" {
		body = tr("Нет текстового слоя. Сканированные страницы требуют OCR.")
	}
	return tr("PDF · текстовый слой\n") + body, nil
}
func htmlPreview(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity
	var b strings.Builder
	hidden := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New(tr("HTML не прочитан."))
		}
		switch v := token.(type) {
		case xml.StartElement:
			if v.Name.Local == "script" || v.Name.Local == "style" {
				hidden++
			}
			if hidden == 0 && (v.Name.Local == "p" || v.Name.Local == "br" || v.Name.Local == "div" || v.Name.Local == "tr") {
				b.WriteByte('\n')
			}
		case xml.EndElement:
			if (v.Name.Local == "script" || v.Name.Local == "style") && hidden > 0 {
				hidden--
			}
		case xml.CharData:
			if hidden == 0 {
				b.WriteString(string(v))
			}
		}
	}
	return tr("HTML · текст без выполнения скриптов\n") + clean(b.String()), nil
}
