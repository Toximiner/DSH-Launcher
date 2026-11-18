package main

import "regexp"

var diagnosticRedactors = []*regexp.Regexp{
	regexp.MustCompile(`(?i)([?&](?:token|access_token|key|api_key|auth|password)=)[^\s&#"'<>]+`),
	regexp.MustCompile(`(?i)(\b(?:Bearer|Basic)\s+)[^\s"'<>]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|apikey|secret|password|passwd|access[_-]?token|auth[_-]?token|token|authorization|cookie)["']?\s*[:=]\s*["']?)[^\r\n"'&,;]+`),
	regexp.MustCompile(`(\b(?:sk|pk|rk)-)[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`(\bgh[pousr]_)[A-Za-z0-9]+`),
	regexp.MustCompile(`(https?://)[^/\s:@]+:[^@\s/]+@`),
}

func redactDiagnostic(text string) string {
	text = clean(text)
	for _, r := range diagnosticRedactors {
		text = r.ReplaceAllString(text, tr("${1}[СКРЫТО]"))
	}
	return text
}
