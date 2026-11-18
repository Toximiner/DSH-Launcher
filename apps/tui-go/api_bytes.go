package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
)

// Only the documented workspace file byte carrier is accepted here. Other
// attachment layouts need their own typed decoder rather than arbitrary paths.
func decodeFileCarrier(contentType string, data []byte) ([]byte, error) {
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil || media != "multipart/form-data" || params["boundary"] == "" {
		return nil, errors.New(tr("Неподдерживаемый бинарный ответ dsh."))
	}
	reader := multipart.NewReader(bytes.NewReader(data), params["boundary"])
	fields := map[string][]byte{}
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil || len(fields) >= 2 {
			return nil, errors.New(tr("Повреждённый бинарный ответ dsh."))
		}
		name := part.FormName()
		if _, exists := fields[name]; exists || name == "" {
			return nil, errors.New(tr("Повторяющееся поле бинарного ответа."))
		}
		value, e := io.ReadAll(part)
		if e != nil {
			return nil, e
		}
		fields[name] = value
	}
	var meta struct {
		Type   string `json:"type"`
		ID     string `json:"rpcId"`
		Result struct {
			OK    bool           `json:"ok"`
			Value map[string]any `json:"value"`
		} `json:"result"`
		Attachments []struct {
			Codec, Part string
			Path        []string
		} `json:"attachments"`
	}
	if json.Unmarshal(fields["metadata"], &meta) != nil || !meta.Result.OK || len(meta.Attachments) != 1 || len(fields) != 2 {
		return nil, errors.New(tr("Неверные метаданные бинарного ответа."))
	}
	a := meta.Attachments[0]
	value, exists := fields[a.Part]
	placeholder, present := meta.Result.Value["data"]
	if a.Codec != "bytes" || a.Part == "metadata" || !exists || len(a.Path) != 1 || a.Path[0] != "data" || !present || placeholder != nil {
		return nil, errors.New(tr("Неверное вложение бинарного ответа."))
	}
	meta.Result.Value["data"] = value
	return json.Marshal(meta)
}
