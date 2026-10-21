package mail

import (
	"strings"
	"testing"
)

func TestParseMessagePlain(t *testing.T) {
	raw := []byte("From: Alice <a@example.com>\r\n" +
		"To: Bob <b@example.com>\r\n" +
		"Subject: =?UTF-8?B?5rWL6K+V?=\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0800\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"hello line1\r\nline2\r\n")
	m, err := ParseMessage(7, raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.UID != 7 {
		t.Fatalf("uid=%d", m.UID)
	}
	if m.Subject != "测试" {
		t.Fatalf("subject=%q", m.Subject)
	}
	if m.Body != "hello line1\nline2\n" {
		t.Fatalf("body=%q", m.Body)
	}
}

func TestAccountDefaults(t *testing.T) {
	a := Account{Email: "x@y.z", IMAPHost: "imap.y.z", SMTPHost: "smtp.y.z", SSL: true}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.IMAPPort != 993 || a.SMTPPort != 587 {
		t.Fatalf("ports %d %d", a.IMAPPort, a.SMTPPort)
	}
}

func TestParseMessageHTML(t *testing.T) {
	from := "From: Bot <bot@x.com>" + "\r\n"
	subj := "Subject: hi" + "\r\n"
	ct := "Content-Type: text/html; charset=utf-8" + "\r\n"
	body := "<html><head><style>p{}</style><script>var a=1;</script></head><body><p>Hello <b>World</b></p><div>Line2</div></body></html>"
	raw := []byte(from + subj + ct + "\r\n" + "\r\n" + body)
	m, err := ParseMessage(1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.Body, "<") || strings.Contains(m.Body, "var a") {
		t.Fatalf("body still markup: %q", m.Body)
	}
	if !strings.Contains(m.Body, "Hello") || !strings.Contains(m.Body, "World") {
		t.Fatalf("body=%q", m.Body)
	}
}
