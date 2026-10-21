// 视图渲染：墨页/微信读书风格列表 + 阅读 + 撰写。
package main

import (
	"fmt"
	"image"
	"strings"

	"c1device"
)

const (
	rowH     = 16
	titleY   = 0
	contentY = rowH
	listRows = 7
	statusY  = contentY + listRows*rowH
)

func (a *app) render() c1device.Frame {
	c := c1device.NewCanvas()
	c.Clear()
	switch a.view {
	case viewGuide:
		c.DrawText(a.face, 2, titleY, "c1mail 邮件")
		drawLines(c, a.face, []string{
			"未找到账号配置",
			"",
			"1. 编辑 account.json",
			"2. 推到设备 data/ 目录",
			"3. 重新启动本应用",
		})
	case viewInbox:
		a.renderInbox(c)
	case viewRead:
		a.renderRead(c)
	case viewCompose:
		a.renderCompose(c)
	case viewOutbox:
		a.renderOutbox(c)
	case viewSettings:
		a.renderSettings(c)
	case viewError:
		c.DrawText(a.face, 2, titleY, "出错")
		drawLines(c, a.face, wrapText(a.errMsg, 280))
		c.DrawText(a.face, 2, statusY, "←/Back 返回")
	}
	return c.Frame(128)
}

func drawLines(c *c1device.Canvas, face *c1device.Face, lines []string) {
	for i, ln := range lines {
		if i >= listRows+1 {
			break
		}
		c.DrawText(face, 2, contentY+i*rowH, ln)
	}
}

func (a *app) renderInbox(c *c1device.Canvas) {
	c.DrawText(a.face, 2, titleY, "收件箱")
	c.DrawTextRight(a.face, c1device.DisplayWidth-2, titleY, fmt.Sprintf("%d 封", len(a.inbox)))
	c.FillRect(image.Rect(2, contentY-2, c1device.DisplayWidth-2, contentY))
	if len(a.inbox) == 0 {
		msg := "空，按 R 同步"
		if a.status != "" {
			msg = a.status
		}
		c.DrawText(a.face, 2, contentY+rowH, msg)
	} else {
		for row := 0; row < listRows; row++ {
			idx := a.inboxSel - a.inboxSel%listRows + row
			if idx >= len(a.inbox) {
				break
			}
			s := a.inbox[idx]
			label := " " + s.Subject
			if label == " " {
				label = " (无主题)"
			}
			label = truncate(a.face, label, c1device.DisplayWidth-4)
			top := contentY + row*rowH
			if idx == a.inboxSel {
				c.DrawInvertedTextBar(a.face, image.Rect(0, top, c1device.DisplayWidth, top+rowH), label)
			} else {
				c.DrawText(a.face, 2, top, label)
			}
		}
		c.DrawScrollIndicator(c1device.DisplayWidth-3, contentY, statusY, a.inboxSel, len(a.inbox))
	}
	hint := "OK阅读 c写信 o发件 s设置"
	if a.message != "" {
		hint = a.message
	}
	c.DrawInvertedTextBar(a.face, image.Rect(0, statusY, c1device.DisplayWidth, c1device.DisplayHeight),
		truncate(a.face, hint, c1device.DisplayWidth-4))
}

func (a *app) renderRead(c *c1device.Canvas) {
	m := a.reading
	title := "(无主题)"
	from := ""
	if m != nil {
		if m.Subject != "" {
			title = m.Subject
		}
		from = m.From
	}
	c.DrawText(a.face, 2, titleY, truncate(a.face, title, c1device.DisplayWidth-40))
	c.DrawTextRight(a.face, c1device.DisplayWidth-2, titleY, "邮件")
	c.FillRect(image.Rect(2, contentY-2, c1device.DisplayWidth-2, contentY))
	if from != "" {
		c.DrawText(a.face, 2, contentY, truncate(a.face, from, c1device.DisplayWidth-4))
	}
	if m != nil {
		lines := wrapText(m.Body, 280)
		top := contentY + rowH
		for i := a.readScroll; i < len(lines) && i < a.readScroll+6; i++ {
			c.DrawText(a.face, 2, top, lines[i])
			top += rowH
		}
	}
	c.DrawInvertedTextBar(a.face, image.Rect(0, statusY, c1device.DisplayWidth, c1device.DisplayHeight),
		"c回复 y复制 ↑↓滚动 ←返回")
}

func (a *app) renderCompose(c *c1device.Canvas) {
	c.DrawText(a.face, 2, titleY, "写信")
	c.DrawTextRight(a.face, c1device.DisplayWidth-2, titleY, "OK发送")
	c.FillRect(image.Rect(2, contentY-2, c1device.DisplayWidth-2, contentY))
	fields := []struct {
		name string
		val  string
	}{
		{"收件人", a.to},
		{"主题", a.subject},
		{"正文", a.body},
	}
	top := contentY
	for i, f := range fields {
		label := f.name + "  " + f.val
		if i == 2 {
			// 正文显示多行
			prefix := "  "
			if a.composeField == 2 {
				prefix = "› "
			}
			c.DrawText(a.face, 2, top, prefix+"正文")
			top += rowH
			lines := wrapText(f.val, 270)
			if len(lines) == 0 {
				lines = []string{""}
			}
			// 显示末尾几行
			start := 0
			if len(lines) > 4 {
				start = len(lines) - 4
			}
			for j := start; j < len(lines) && j < start+4; j++ {
				c.DrawText(a.face, 8, top, truncate(a.face, lines[j], 270))
				top += rowH
			}
			break
		}
		prefix := "  "
		if a.composeField == i {
			prefix = "› "
		}
		text := prefix + label
		if a.composeField == i {
			c.DrawInvertedTextBar(a.face, image.Rect(0, top, c1device.DisplayWidth, top+rowH),
				truncate(a.face, text, c1device.DisplayWidth-4))
		} else {
			c.DrawText(a.face, 2, top, truncate(a.face, text, c1device.DisplayWidth-4))
		}
		top += rowH
	}
	hint := "OK发送 p粘贴 ←返回 ↑↓字段"
	if a.message != "" {
		hint = a.message
	}
	c.DrawInvertedTextBar(a.face, image.Rect(0, statusY, c1device.DisplayWidth, c1device.DisplayHeight),
		truncate(a.face, hint, c1device.DisplayWidth-4))
}

func (a *app) renderOutbox(c *c1device.Canvas) {
	c.DrawText(a.face, 2, titleY, "发件箱")
	c.DrawTextRight(a.face, c1device.DisplayWidth-2, titleY, fmt.Sprintf("%d 封", len(a.outbox)))
	c.FillRect(image.Rect(2, contentY-2, c1device.DisplayWidth-2, contentY))
	if len(a.outbox) == 0 {
		c.DrawText(a.face, 2, contentY+rowH, "空")
	} else {
		for row := 0; row < listRows; row++ {
			idx := a.outboxSel - a.outboxSel%listRows + row
			if idx >= len(a.outbox) {
				break
			}
			it := a.outbox[idx]
			label := " " + it.Subject + " → " + it.To
			if it.Error != "" {
				label += " !"
			}
			top := contentY + row*rowH
			if idx == a.outboxSel {
				c.DrawInvertedTextBar(a.face, image.Rect(0, top, c1device.DisplayWidth, top+rowH),
					truncate(a.face, label, c1device.DisplayWidth-4))
			} else {
				c.DrawText(a.face, 2, top, truncate(a.face, label, c1device.DisplayWidth-4))
			}
		}
	}
	hint := "OK重发 ←返回"
	if a.message != "" {
		hint = a.message
	}
	c.DrawInvertedTextBar(a.face, image.Rect(0, statusY, c1device.DisplayWidth, c1device.DisplayHeight),
		truncate(a.face, hint, c1device.DisplayWidth-4))
}

func (a *app) renderSettings(c *c1device.Canvas) {
	c.DrawText(a.face, 2, titleY, "设置")
	c.FillRect(image.Rect(2, contentY-2, c1device.DisplayWidth-2, contentY))
	lines := []string{"账号见 data/account.json"}
	if a.acct != nil {
		lines = []string{
			"邮箱  " + a.acct.Email,
			"IMAP  " + a.acct.IMAPHost,
			"SMTP  " + a.acct.SMTPHost,
		}
	}
	drawLines(c, a.face, lines)
	c.DrawInvertedTextBar(a.face, image.Rect(0, statusY, c1device.DisplayWidth, c1device.DisplayHeight),
		"←返回")
}

func truncate(face *c1device.Face, s string, width int) string {
	if face.Measure(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && face.Measure(string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return "…"
	}
	return string(runes) + "…"
}

func wrapText(s string, widthPx int) []string {
	var lines []string
	for _, raw := range strings.Split(s, "\n") {
		if raw == "" {
			lines = append(lines, "")
			continue
		}
		current := ""
		w := 0
		for _, r := range raw {
			rw := 16
			if r < 0x80 {
				rw = 8
			}
			if w+rw > widthPx && current != "" {
				lines = append(lines, current)
				current = ""
				w = 0
			}
			current += string(r)
			w += rw
		}
		lines = append(lines, current)
	}
	return lines
}
