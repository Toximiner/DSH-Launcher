package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFileByteCarrierRejectsInvalidAttachments(t *testing.T) {
	for _, mutation := range []string{"", "path", "codec", "placeholder", "extra", "duplicate"} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		value := map[string]any{"data": nil, "version": "v1"}
		attachment := map[string]any{"path": []string{"data"}, "codec": "bytes", "part": "bytes0"}
		if mutation == "path" {
			attachment["path"] = []string{"secret"}
		}
		if mutation == "codec" {
			attachment["codec"] = "javascript"
		}
		if mutation == "placeholder" {
			value["data"] = "already filled"
		}
		field, _ := writer.CreateFormField("metadata")
		json.NewEncoder(field).Encode(map[string]any{"type": "server-response", "rpcId": "request", "result": map[string]any{"ok": true, "value": value}, "attachments": []any{attachment}})
		file, _ := writer.CreateFormFile("bytes0", "file")
		file.Write([]byte{0, 1, 255})
		if mutation == "extra" || mutation == "duplicate" {
			name := "extra"
			if mutation == "duplicate" {
				name = "bytes0"
			}
			p, _ := writer.CreateFormField(name)
			p.Write([]byte("unexpected"))
		}
		writer.Close()
		data, err := decodeFileCarrier(writer.FormDataContentType(), body.Bytes())
		if mutation != "" {
			if err == nil {
				t.Fatalf("accepted %s attachment", mutation)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Result struct{ Value struct{ Data []byte } }
		}
		if json.Unmarshal(data, &decoded) != nil || !bytes.Equal(decoded.Result.Value.Data, []byte{0, 1, 255}) {
			t.Fatal("file bytes changed in the carrier")
		}
	}
}
func TestDocumentPreviewsAreBoundedAndPassive(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 8))
	img.Set(0, 0, color.White)
	var data bytes.Buffer
	png.Encode(&data, img)
	preview, err := imagePreview(data.Bytes())
	if err != nil || !strings.Contains(preview, "10 × 8") || len(strings.Split(ansi.Strip(preview), "\n")) > 16 {
		t.Fatal("image thumbnail invalid", err)
	}
	if _, err = imagePreview([]byte("invalid")); err == nil {
		t.Fatal("accepted corrupt image")
	}
	html, err := htmlPreview([]byte("<html><body><p>Hello &amp; мир</p><script>secret()</script><style>hidden</style></body></html>"))
	if err != nil || !strings.Contains(html, "Hello & мир") || strings.Contains(html, "secret") || strings.Contains(html, "hidden") {
		t.Fatal("HTML preview included active content", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = pdfPreview(ctx, []byte("invalid")); err == nil {
		t.Fatal("cancelled PDF conversion succeeded")
	}
}
