package service

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// Everything that ends up inside an email goes through esc() first, so a
// buyer who types <script> as their name just sees <script> in the email.
func esc(s string) string { return html.EscapeString(s) }

// oneLine makes user text safe for a subject line or name field.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// paragraphs turns plain text typed by an admin into email-safe HTML.
// A blank line starts a new paragraph, a single line break stays a line
// break, and bare links become clickable.
func paragraphs(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out strings.Builder
	for _, block := range strings.Split(strings.TrimSpace(text), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		safe := esc(block)
		safe = urlPattern.ReplaceAllStringFunc(safe, func(u string) string {
			return fmt.Sprintf(`<a href="%s" style="color:#4F4FA0">%s</a>`, u, u)
		})
		safe = strings.ReplaceAll(safe, "\n", "<br>")
		out.WriteString(`<p style="margin:0 0 16px;line-height:1.65">`)
		out.WriteString(safe)
		out.WriteString(`</p>`)
	}
	return out.String()
}

// emailPage wraps a message body in one plain, readable layout used by every
// email the site sends. footerHTML is optional small print.
func emailPage(heading, bodyHTML, footerHTML string) string {
	return `<!doctype html><html><body style="margin:0;background:#F0EFED;padding:24px 12px;font-family:Arial,Helvetica,sans-serif;color:#0B0C0E">` +
		`<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">` +
		`<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;background:#ffffff;border-radius:12px">` +
		`<tr><td style="height:5px;background:linear-gradient(90deg,#1C98D8,#6D6FC4,#C45D96);border-radius:12px 12px 0 0"></td></tr>` +
		`<tr><td style="padding:32px 32px 8px;font-size:13px;letter-spacing:.14em;text-transform:uppercase;color:#555">Akintola Samuel</td></tr>` +
		`<tr><td style="padding:0 32px 8px;font-size:24px;font-weight:700">` + esc(heading) + `</td></tr>` +
		`<tr><td style="padding:12px 32px 28px;font-size:16px">` + bodyHTML + `</td></tr>` +
		footerHTML +
		`</table></td></tr></table></body></html>`
}

func emailFooter(inner string) string {
	return `<tr><td style="padding:16px 32px 28px;border-top:1px solid #eee;font-size:12px;line-height:1.6;color:#666">` + inner + `</td></tr>`
}

func emailButton(href, label string) string {
	return `<p style="margin:24px 0"><a href="` + esc(href) + `" style="display:inline-block;background:#4F4FA0;color:#ffffff;text-decoration:none;padding:14px 26px;border-radius:999px;font-weight:700">` + esc(label) + `</a></p>`
}

func firstName(full string) string {
	full = strings.TrimSpace(full)
	if i := strings.IndexByte(full, ' '); i > 0 {
		return full[:i]
	}
	return full
}
