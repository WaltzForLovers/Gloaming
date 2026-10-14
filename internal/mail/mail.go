package mail

import (
	"context"
	"mime"
	"net/smtp"
	"strings"
)

type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

type SMTP struct {
	addr string
	from string
}

func Open(addr, from string) *SMTP {
	return &SMTP{addr: addr, from: from}
}

func (s *SMTP) Send(_ context.Context, to, subject, body string) error {
	if s == nil || s.addr == "" || to == "" {
		return nil
	}
	msg := strings.Join([]string{
		"From: " + s.from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	return smtp.SendMail(s.addr, nil, s.from, []string{to}, []byte(msg))
}
