package auth

import (
	"strings"
	"text/template"
)

type mailTemplate struct {
	subject *template.Template
	body    *template.Template
}

type mailData struct {
	DisplayName string
	Username    string
	Link        string
	BaseURL     string
}

func mustTemplate(subject, body string) mailTemplate {
	return mailTemplate{
		subject: template.Must(template.New("subject").Parse(subject)),
		body:    template.Must(template.New("body").Parse(body)),
	}
}

func (t mailTemplate) render(data mailData) (string, string, error) {
	var subject, body strings.Builder
	if err := t.subject.Execute(&subject, data); err != nil {
		return "", "", err
	}
	if err := t.body.Execute(&body, data); err != nil {
		return "", "", err
	}
	return subject.String(), body.String(), nil
}

var verifyTemplate = mustTemplate(
	"Verify your hyl email",
	`Hi {{.DisplayName}},

confirm this address to finish setting up your hyl account:

{{.Link}}

The link is valid for 24 hours and can only be used once. If you did not
create an account, you can ignore this message.

— hyl ({{.BaseURL}})
`)

var resetTemplate = mustTemplate(
	"Reset your hyl password",
	`Hi {{.DisplayName}},

someone asked to reset the password for the hyl account "{{.Username}}".

{{.Link}}

The link is valid for 1 hour and can only be used once. Your current password
keeps working until you set a new one. If this was not you, ignore this
message.

— hyl ({{.BaseURL}})
`)
