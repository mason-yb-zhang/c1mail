// C1-Slim 邮件客户端：纯文本收发 + 本地缓存。
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"c1device"
	"c1mail/mail"
)

//go:embed assets/pkg-font.bin
var fontData []byte

type viewMode int

const (
	viewGuide viewMode = iota
	viewInbox
	viewRead
	viewCompose
	viewOutbox
	viewSettings
	viewError
)

type app struct {
	face  *c1device.Face
	acct  *mail.Account
	cache *mail.Cache

	view    viewMode
	status  string
	errMsg  string
	message string

	inbox      mail.HeaderList
	inboxSel   int
	reading    *mail.Message
	readScroll int

	to, subject, body string
	composeField      int
	composeCursor     int

	outbox    []mail.OutboxItem
	outboxSel int

	settingsSel int
	syncing     bool
	syncDone    chan error
}

func writeTrace(format string, args ...any) {
	f, err := os.OpenFile(dataDir()+"/c1mail.trace", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"\n", args...)
}

func main() {
	debug.SetMemoryLimit(16 << 20)
	debug.SetGCPercent(50)
	defer func() {
		if r := recover(); r != nil {
			writeTrace("panic: %v\n%s", r, debug.Stack())
			panic(r)
		}
	}()
	signal.Ignore(syscall.SIGHUP)
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		writeTrace("signal %v", sig)
		os.Exit(0)
	}()

	face, err := c1device.NewBitmapFace(fontData, 16)
	if err != nil {
		fmt.Fprintln(os.Stderr, "c1mail: font:", err)
		os.Exit(1)
	}
	defer face.Close()

	if err := ensureDataDir(); err != nil {
		fmt.Fprintln(os.Stderr, "c1mail: data:", err)
		os.Exit(1)
	}
	acct, err := mail.LoadAccount(dataDir() + "/account.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "c1mail: account:", err)
	}
	cache := mail.NewCache(dataDir() + "/cache")
	inbox, _ := cache.LoadInbox()
	outbox, _ := cache.ListOutbox()

	a := &app{face: face, acct: acct, cache: cache, inbox: inbox, outbox: outbox}
	if acct == nil {
		a.view = viewGuide
	} else {
		a.view = viewInbox
	}

	platform, err := c1device.OpenPlatform()
	if err != nil {
		fmt.Fprintln(os.Stderr, "c1mail: platform:", err)
		os.Exit(1)
	}
	defer platform.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	drawCount := 0
	draw := func(full bool) {
		drawCount++
		_ = platform.Draw(a.render(), full)
	}
	draw(true)

	if a.acct != nil {
		go a.syncInbox()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-platform.Events():
			if !ok {
				return
			}
			if ev.Repeat && ev.Key != c1device.KeyUp && ev.Key != c1device.KeyDown {
				continue
			}
			ev.Key = a.orientation().RemapKey(ev.Key)
			if a.handleEvent(ev, cancel) {
				return
			}
			draw(drawCount%8 == 0)
		case err := <-a.syncDone:
			a.syncing = false
			a.syncDone = nil
			if err != nil {
				writeTrace("sync err: %v", err)
				a.message = err.Error()
			} else {
				writeTrace("sync ok: %d", len(a.inbox))
				a.message = fmt.Sprintf("已同步 %d 封", len(a.inbox))
			}
			draw(true)
		case <-time.After(500 * time.Millisecond):
			if a.syncing {
				draw(false)
			}
		}
	}
}

func (a *app) orientation() c1device.Orientation { return c1device.Rotate0 }

func (a *app) handleEvent(ev c1device.Event, cancel context.CancelFunc) bool {
	switch a.view {
	case viewGuide:
		if ev.Key == c1device.KeyBack || ev.Key == c1device.KeyOK {
			cancel()
			return true
		}
	case viewInbox:
		return a.handleInbox(ev, cancel)
	case viewRead:
		a.handleRead(ev)
	case viewCompose:
		a.handleCompose(ev)
	case viewOutbox:
		a.handleOutbox(ev)
	case viewSettings:
		a.handleSettings(ev)
	case viewError:
		if ev.Key == c1device.KeyBack || ev.Key == c1device.KeyOK || ev.Key == c1device.KeyLeft {
			a.view = viewInbox
		}
	}
	return false
}

func isRune(ev c1device.Event, r rune) bool {
	return ev.Key == c1device.KeyRune && ev.Rune == r
}

func (a *app) handleInbox(ev c1device.Event, cancel context.CancelFunc) bool {
	n := len(a.inbox)
	switch {
	case ev.Key == c1device.KeyUp && n > 0 && a.inboxSel > 0:
		a.inboxSel--
	case ev.Key == c1device.KeyDown && n > 0 && a.inboxSel < n-1:
		a.inboxSel++
	case ev.Key == c1device.KeyOK, ev.Key == c1device.KeyRight:
		if n == 0 {
			a.syncInbox()
			return false
		}
		a.openMessage(a.inbox[a.inboxSel].UID)
	case isRune(ev, 'c'):
		a.to, a.subject, a.body = "", "", ""
		a.composeField, a.composeCursor = 0, 0
		a.view = viewCompose
	case isRune(ev, 'o'):
		a.outbox, _ = a.cache.ListOutbox()
		a.view = viewOutbox
	case isRune(ev, 's'):
		a.view = viewSettings
	case isRune(ev, 'r'):
		a.syncInbox()
	case ev.Key == c1device.KeyBack, ev.Key == c1device.KeyLeft:
		cancel()
		return true
	}
	return false
}

func (a *app) handleRead(ev c1device.Event) {
	switch {
	case ev.Key == c1device.KeyUp:
		if a.readScroll > 0 {
			a.readScroll--
		}
	case ev.Key == c1device.KeyDown:
		a.readScroll++
	case isRune(ev, 'y') && a.reading != nil:
		a.copyReading()
	case isRune(ev, 'c') && a.reading != nil:
		a.to = a.reading.From
		a.subject = replySubject(a.reading.Subject)
		a.body = "\n\n> " + strings.ReplaceAll(trimBody(a.reading.Body, 400), "\n", "\n> ")
		a.composeField = 2
		a.composeCursor = len(a.body)
		a.view = viewCompose
	case ev.Key == c1device.KeyBack, ev.Key == c1device.KeyLeft:
		a.view = viewInbox
	}
}

func (a *app) handleCompose(ev c1device.Event) {
	switch {
	case ev.Key == c1device.KeyBack:
		a.view = viewInbox
	case ev.Key == c1device.KeyLeft:
		if a.composeField == 2 && a.composeCursor > 0 {
			a.composeCursor--
		} else if a.composeField > 0 {
			a.composeField--
		}
	case ev.Key == c1device.KeyRight:
		if a.composeField == 2 {
			if a.composeCursor < len(a.body) {
				a.composeCursor++
			}
		} else if a.composeField < 2 {
			a.composeField++
		}
	case ev.Key == c1device.KeyUp:
		if a.composeField > 0 {
			a.composeField--
		}
	case ev.Key == c1device.KeyDown:
		if a.composeField < 2 {
			a.composeField++
		}
	case ev.Key == c1device.KeyOK:
		a.sendOrQueue()
	case isRune(ev, '\b'):
		a.composeBackspace()
	case ev.Key == c1device.KeyRune:
		a.composeInsert(ev.Rune)
	}
}

func (a *app) handleOutbox(ev c1device.Event) {
	n := len(a.outbox)
	switch {
	case ev.Key == c1device.KeyUp && n > 0 && a.outboxSel > 0:
		a.outboxSel--
	case ev.Key == c1device.KeyDown && n > 0 && a.outboxSel < n-1:
		a.outboxSel++
	case ev.Key == c1device.KeyOK:
		if n > 0 {
			a.flushOutboxItem(a.outbox[a.outboxSel])
		}
	case ev.Key == c1device.KeyBack, ev.Key == c1device.KeyLeft:
		a.view = viewInbox
	}
}

func (a *app) handleSettings(ev c1device.Event) {
	if ev.Key == c1device.KeyBack || ev.Key == c1device.KeyLeft {
		a.view = viewInbox
	}
}

func (a *app) composeInsert(r rune) {
	switch a.composeField {
	case 0:
		a.to += string(r)
	case 1:
		a.subject += string(r)
	case 2:
		if a.composeCursor > len(a.body) {
			a.composeCursor = len(a.body)
		}
		a.body = a.body[:a.composeCursor] + string(r) + a.body[a.composeCursor:]
		a.composeCursor++
	}
}

func (a *app) composeBackspace() {
	switch a.composeField {
	case 0:
		if a.to != "" {
			a.to = a.to[:len(a.to)-1]
		}
	case 1:
		if a.subject != "" {
			a.subject = a.subject[:len(a.subject)-1]
		}
	case 2:
		if a.composeCursor > 0 && len(a.body) > 0 {
			a.body = a.body[:a.composeCursor-1] + a.body[a.composeCursor:]
			a.composeCursor--
		}
	}
}

func (a *app) sendOrQueue() {
	if a.acct == nil {
		a.message = "未配置账号"
		return
	}
	item := mail.OutboxItem{To: a.to, Subject: a.subject, Body: a.body}
	go func() {
		if err := mail.SendMail(a.acct, a.to, a.subject, a.body); err != nil {
			item.Error = err.Error()
			_ = a.cache.SaveOutbox(item)
			a.message = "发送失败，已入发件箱"
		} else {
			a.message = "已发送"
			a.view = viewInbox
		}
	}()
}

func (a *app) flushOutboxItem(item mail.OutboxItem) {
	if a.acct == nil {
		return
	}
	go func() {
		if err := mail.SendMail(a.acct, item.To, item.Subject, item.Body); err != nil {
			item.Error = err.Error()
			_ = a.cache.SaveOutbox(item)
			a.message = "发送失败: " + err.Error()
		} else {
			_ = a.cache.RemoveOutbox(item.ID)
			a.outbox, _ = a.cache.ListOutbox()
			a.message = "已发送"
		}
	}()
}

func (a *app) openMessage(uid uint32) {
	if m, err := a.cache.LoadMessage(uid); err == nil {
		a.reading = m
		a.readScroll = 0
		a.view = viewRead
		return
	}
	if a.acct == nil {
		a.errMsg = "未配置账号"
		a.view = viewError
		return
	}
	a.status = "下载中…"
	go func() {
		c, err := mail.IMAPDial(a.acct.IMAPHost, a.acct.IMAPPort, a.acct.IMAPImplicitTLS(), a.acct.SkipTLS)
		if err != nil {
			a.errMsg = err.Error()
			a.view = viewError
			return
		}
		defer c.Close()
		if err := c.Login(a.acct.Email, a.acct.Password); err != nil {
			a.errMsg = err.Error()
			a.view = viewError
			return
		}
		if _, err := c.Select("INBOX"); err != nil {
			a.errMsg = err.Error()
			a.view = viewError
			return
		}
		raw, err := c.UIDFetchBody(uid)
		if err != nil {
			a.errMsg = err.Error()
			a.view = viewError
			return
		}
		m, err := mail.ParseMessage(uid, raw)
		if err != nil {
			a.errMsg = err.Error()
			a.view = viewError
			return
		}
		_ = a.cache.SaveMessage(m)
		a.reading = m
		a.readScroll = 0
		a.status = ""
		a.view = viewRead
	}()
}

func (a *app) syncInbox() {
	if a.acct == nil || a.syncing {
		return
	}
	a.syncing = true
	a.syncDone = make(chan error, 1)
	go func() {
		c, err := mail.IMAPDial(a.acct.IMAPHost, a.acct.IMAPPort, a.acct.IMAPImplicitTLS(), a.acct.SkipTLS)
		if err != nil {
			a.syncDone <- err
			return
		}
		defer c.Close()
		if err := c.Login(a.acct.Email, a.acct.Password); err != nil {
			a.syncDone <- err
			return
		}
		total, err := c.Select("INBOX")
		if err != nil {
			a.syncDone <- err
			return
		}
		list, err := c.UIDFetchHeaders(total, 50)
		if err != nil {
			a.syncDone <- err
			return
		}
		a.inbox = list
		_ = a.cache.SaveInbox(list)
		a.syncDone <- nil
	}()
}

func replySubject(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "re:") {
		return s
	}
	return "Re: " + s
}

func trimBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// copyReading 把当前邮件（含信头摘要）写入共享剪贴板。
func (a *app) copyReading() {
	if a.reading == nil {
		return
	}
	m := a.reading
	text := "Subject: " + m.Subject + "\nFrom: " + m.From + "\nDate: " + m.Date.Format("2006-01-02 15:04") + "\n\n" + m.Body
	if err := mail.WriteClipboard(text); err != nil {
		a.message = "复制失败"
	} else {
		a.message = "已复制到剪贴板"
	}
}

// pasteToCompose 从共享剪贴板粘贴到当前字段。
func (a *app) pasteToCompose() {
	text := mail.ReadClipboard()
	if text == "" {
		a.message = "剪贴板为空"
		return
	}
	switch a.composeField {
	case 0:
		a.to += text
	case 1:
		a.subject += strings.ReplaceAll(text, "\n", " ")
	case 2:
		if a.composeCursor > len(a.body) {
			a.composeCursor = len(a.body)
		}
		a.body = a.body[:a.composeCursor] + text + a.body[a.composeCursor:]
		a.composeCursor += len(text)
	}
	a.message = "已粘贴"
}
