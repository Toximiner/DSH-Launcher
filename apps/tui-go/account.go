package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

func accountClient() map[string]any {
	_, offset := time.Now().Zone()
	locale := "ru"
	if activeEnglish.Load() {
		locale = "en"
	}
	return map[string]any{"version": version, "locale": locale, "timezoneOffsetSeconds": offset}
}

func readAccount(ctx context.Context, api *dshAPI) ([]benchRow, error) {
	var state map[string]any
	if err := api.rpc(ctx, "account/getState", map[string]any{}, &state); err != nil {
		return nil, err
	}
	status := stringValue(state, "status")
	if status != "signed-out" && status != "credential-stored" {
		return nil, errors.New(tr("Неподдерживаемое состояние аккаунта."))
	}
	title := tr("Вход не выполнен")
	if status == "credential-stored" {
		title = tr("Учётные данные сохранены")
	}
	body := tr("Ctrl+N — вход через браузер · Ctrl+X — выход\nF3 — профиль и баланс · F5 — обновить\nВ SSH используйте туннель к порту dsh для обратного вызова.")
	if attempt := objectValue(state["attempt"]); attempt != nil {
		body += tr("\nВход: ") + clean(stringValue(attempt, "phase"))
		if link := safeAccountURL(stringValue(attempt, "authorizeUrl")); link != "" {
			body += tr("\nОткройте в браузере:\n") + link
		}
		if code := stringValue(attempt, "errorCode"); code != "" {
			body += tr("\nОшибка: ") + clean(code)
		}
		body += tr("\nCtrl+G — отменить попытку входа")
	}
	return []benchRow{{ID: "account", Title: title, Body: body, Data: state}}, nil
}

func safeAccountURL(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func accountDetails(ctx context.Context, api *dshAPI) (string, error) {
	args := map[string]any{"client": accountClient()}
	var profile, balance any
	if err := api.rpc(ctx, "account/getProfile", args, &profile); err != nil {
		return "", err
	}
	if err := api.rpc(ctx, "account/getBalance", args, &balance); err != nil {
		return "", err
	}
	return fmt.Sprintf(tr("Профиль:\n%s\n\nБаланс:\n%s"), prettyValue(profile), prettyValue(balance)), nil
}
