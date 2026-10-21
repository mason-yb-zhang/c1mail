package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"c1device"
	"c1mail/mail"
)

func TestPreview(t *testing.T) {
	face, err := c1device.NewBitmapFace(fontData, 16)
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()

	inbox := mail.HeaderList{
		{UID: 1, From: "boss@example.com", Subject: "周报请查收", Date: time.Now()},
		{UID: 2, From: "ci@example.com", Subject: "Build failed", Date: time.Now().Add(-time.Hour)},
		{UID: 3, From: "me@example.com", Subject: "会议纪要", Date: time.Now().Add(-2 * time.Hour)},
	}
	cases := []struct {
		name string
		app  *app
	}{
		{"inbox", &app{face: face, view: viewInbox, inbox: inbox, inboxSel: 1}},
		{"read", &app{
			face: face, view: viewRead, readScroll: 0,
			reading: &mail.Message{
				UID: 1, From: "boss@example.com", Subject: "周报请查收",
				Body: "本周完成邮件客户端原型。\n请补充测试结论。\n谢谢。",
			},
		}},
		{"compose", &app{
			face: face, view: viewCompose,
			to: "team@example.com", subject: "进度", body: "今天完成了收发。",
			composeField: 2, composeCursor: 12,
		}},
		{"outbox", &app{
			face: face, view: viewOutbox,
			outbox:    []mail.OutboxItem{{To: "a@b.c", Subject: "待发", Body: "x"}},
			outboxSel: 0,
		}},
		{"settings", &app{
			face: face, view: viewSettings,
			acct: &mail.Account{Email: "me@example.com", IMAPHost: "imap.example.com", SMTPHost: "smtp.example.com"},
		}},
	}
	outDir := filepath.Join("..", "..", "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		frame := tc.app.render()
		if len(frame) != c1device.FrameBytes {
			t.Errorf("%s: frame = %d bytes", tc.name, len(frame))
		}
		path := filepath.Join(outDir, "preview-"+tc.name+".frame")
		if err := os.WriteFile(path, frame[:], 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
