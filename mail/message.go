// 邮件消息：元数据 + 纯文本正文。
package mail

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// Message 一封邮件（纯文本）。
type Message struct {
	UID       uint32    `json:"uid"`
	MessageID string    `json:"messageId,omitempty"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	Date      time.Time `json:"date"`
	Body      string    `json:"body"`
	Size      int       `json:"size,omitempty"`
	Read      bool      `json:"read"`
}

// Summary 列表项（不含正文）。
type Summary struct {
	UID     uint32    `json:"uid"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Date    time.Time `json:"date"`
	Size    int       `json:"size,omitempty"`
	Read    bool      `json:"read"`
}

// HeaderList 主题列表（同步索引用）。
type HeaderList []Summary

// ParseMessage 解析 RFC822 字节为纯文本 Message（忽略 HTML/附件）。
func ParseMessage(uid uint32, raw []byte) (*Message, error) {
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("解析邮件: %w", err)
	}
	msg := &Message{
		UID:       uid,
		MessageID: strings.TrimSpace(m.Header.Get("Message-Id")),
		From:      m.Header.Get("From"),
		To:        m.Header.Get("To"),
		Subject:   decodeHeader(m.Header.Get("Subject")),
	}
	if ds := m.Header.Get("Date"); ds != "" {
		if t, err := mail.ParseDate(ds); err == nil {
			msg.Date = t
		}
	}
	if msg.Date.IsZero() {
		msg.Date = time.Now()
	}
	body, err := plainBody(m)
	if err != nil {
		return nil, err
	}
	msg.Body = body
	msg.Size = len(raw)
	return msg, nil
}

func decodeHeader(s string) string {
	if s == "" {
		return ""
	}
	dec := &mime.WordDecoder{
		CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
			switch strings.ToLower(charset) {
			case "gbk", "gb2312", "gb18030":
				return simplifiedchinese.GBK.NewDecoder().Reader(input), nil
			case "big5":
				return traditionalchinese.Big5.NewDecoder().Reader(input), nil
			}
			return input, nil
		},
	}
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return out
}

// plainBody 递归找 text/plain；找不到则退化为空。
func plainBody(m *mail.Message) (string, error) {
	ct := m.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain; charset=utf-8"
	}
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType, params = "text/plain", map[string]string{"charset": "utf-8"}
	}
	if mediaType == "text/plain" {
		raw, err := decodeText(m.Body, m.Header.Get("Content-Transfer-Encoding"), params["charset"])
		if err != nil {
			return "", err
		}
		return stripHTMLIfMarkup(raw), nil
	}
	if mediaType == "text/html" {
		raw, err := decodeText(m.Body, m.Header.Get("Content-Transfer-Encoding"), params["charset"])
		if err != nil {
			return "", err
		}
		return stripHTML(raw), nil
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return "", nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", nil
	}
	// 简易 multipart 遍历
	raw := readAll(m.Body)
	parts := splitMultipart(raw, boundary)
	var htmlFallback string
	for _, p := range parts {
		sub, err := mail.ReadMessage(strings.NewReader(p))
		if err != nil {
			continue
		}
		sct := sub.Header.Get("Content-Type")
		if sct == "" {
			sct = "text/plain"
		}
		smt, sp, _ := mime.ParseMediaType(sct)
		if smt == "text/plain" {
			raw, err := decodeText(sub.Body, sub.Header.Get("Content-Transfer-Encoding"), sp["charset"])
			if err == nil && strings.TrimSpace(raw) != "" {
				return stripHTMLIfMarkup(raw), nil
			}
		}
		if smt == "text/html" && htmlFallback == "" {
			h, err := decodeText(sub.Body, sub.Header.Get("Content-Transfer-Encoding"), sp["charset"])
			if err == nil {
				htmlFallback = h
			}
		}
	}
	if htmlFallback != "" {
		return stripHTML(htmlFallback), nil
	}
	return "", nil
}

// stripHTMLIfMarkup 疑似 HTML 时去标签。
func stripHTMLIfMarkup(s string) string {
	low := strings.ToLower(s)
	if strings.Contains(low, "<html") || strings.Contains(low, "<!doctype") ||
		strings.Contains(low, "<div") || strings.Contains(low, "<table") ||
		strings.Contains(low, "<p>") || strings.Contains(low, "<span") {
		return stripHTML(s)
	}
	return s
}

// stripHTML 粗暴去标签，只留可读文本。
func stripHTML(s string) string {
	// 去掉 script/style 整块
	low := strings.ToLower(s)
	for _, tag := range []string{"script", "style"} {
		for {
			a := strings.Index(low, "<"+tag)
			if a < 0 {
				break
			}
			b := strings.Index(low[a:], "</"+tag+">")
			if b < 0 {
				s = s[:a]
				break
			}
			end := a + b + len(tag) + 3
			s = s[:a] + s[end:]
			low = strings.ToLower(s)
		}
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, tag := range []string{"<br", "<br/>", "<br />", "</p>", "</div>", "</tr>", "</h1>", "</h2>", "</h3>"} {
		s = strings.ReplaceAll(s, tag, "\n")
		s = strings.ReplaceAll(s, strings.ToUpper(tag), "\n")
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&nbsp;", " ")
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&quot;", "\"")
	lines := strings.Split(out, "\n")
	var kept []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			kept = append(kept, ln)
		}
	}
	return strings.Join(kept, "\n")
}

func readAll(r io.Reader) string {
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return b.String()
}

func splitMultipart(raw, boundary string) []string {
	delim := "--" + boundary
	var parts []string
	for _, c := range strings.Split(raw, delim) {
		c = strings.TrimPrefix(c, "\r\n")
		c = strings.TrimPrefix(c, "\n")
		if c == "" || strings.HasPrefix(c, "--") {
			continue
		}
		if i := strings.Index(c, "\r\n--"); i >= 0 {
			c = c[:i]
		}
		parts = append(parts, c)
	}
	return parts
}

func decodeText(body io.Reader, transferEnc, charset string) (string, error) {
	data := readAll(body)
	transferEnc = strings.ToLower(strings.TrimSpace(transferEnc))
	switch transferEnc {
	case "base64":
		clean := strings.Map(func(r rune) rune {
			if r == 13 || r == 10 {
				return -1
			}
			return r
		}, data)
		dec, err := base64.StdEncoding.DecodeString(clean)
		if err == nil {
			data = string(dec)
		}
	case "quoted-printable":
		if dec, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(data))); err == nil {
			data = string(dec)
		}
	}
	data = strings.ReplaceAll(data, "\r\n", "\n")
	return data, nil
}
