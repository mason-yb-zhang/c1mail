// 极简 IMAP4rev1 客户端：LOGIN / SELECT / UID FETCH / UID SEARCH。
package mail

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

// IMAP 最小客户端。
type IMAP struct {
	conn   net.Conn
	r      *bufio.Reader
	tag    int
	greeting string
}

// IMAPDial 连接 IMAP 服务器。ssl=true 走隐式 TLS。
func IMAPDial(host string, port int, ssl bool, skipVerify bool) (*IMAP, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	var err error
	if ssl {
		cfg := loadRootCAs(skipVerify)
		cfg.ServerName = host
		conn, err = tls.DialWithDialer(&dialer, "tcp", addr, cfg)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("连接 IMAP %s: %w", addr, err)
	}
	c := &IMAP{conn: conn, r: bufio.NewReader(conn)}
	line, err := c.readLine()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读问候: %w", err)
	}
	c.greeting = line
	if !strings.Contains(line, "OK") {
		conn.Close()
		return nil, fmt.Errorf("IMAP 问候异常: %s", line)
	}
	return c, nil
}

func (c *IMAP) readLine() (string, error) {
	line, err := c.r.ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

func (c *IMAP) cmd(format string, args ...any) (string, error) {
	return c.cmdDeadline(25*time.Second, format, args...)
}

func (c *IMAP) cmdDeadline(d time.Duration, format string, args ...any) (string, error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
	c.tag++
	tag := fmt.Sprintf("A%03d", c.tag)
	payload := fmt.Sprintf(format, args...)
	if _, err := fmt.Fprintf(c.conn, "%s %s\r\n", tag, payload); err != nil {
		return "", err
	}
	var sb strings.Builder
	for {
		line, err := c.readLine()
		if err != nil {
			return sb.String(), err
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
		if strings.HasPrefix(line, tag+" ") {
			return sb.String(), nil
		}
	}
}

func ok(resp string) error {
	last := ""
	for _, ln := range strings.Split(resp, "\n") {
		if strings.TrimSpace(ln) != "" {
			last = strings.TrimSpace(ln)
		}
	}
	if strings.Contains(last, "OK") {
		return nil
	}
	return fmt.Errorf("IMAP: %s", last)
}

// Login 账号密码登录，并发送 ID（网易等国内服务常要求）。
func (c *IMAP) Login(user, pass string) error {
	resp, err := c.cmd("LOGIN %s %s", quoteIMAP(user), quoteIMAP(pass))
	if err != nil {
		return err
	}
	if err := ok(resp); err != nil {
		return err
	}
	// 部分国内邮箱（126/163）无 ID 会报 Unsafe Login。
	_, _ = c.cmd(`ID ("name" "c1mail" "version" "1.0" "vendor" "c1-slim" "support" "mailto:support@localhost")`)
	return nil
}

// Select 选中邮箱（默认 INBOX）。
func (c *IMAP) Select(mailbox string) (count int, err error) {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	resp, err := c.cmd("SELECT %s", quoteIMAP(mailbox))
	if err != nil {
		return 0, err
	}
	if err := ok(resp); err != nil {
		return 0, err
	}
	for _, ln := range strings.Split(resp, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "* ") && strings.Contains(ln, "EXISTS") {
			fields := strings.Fields(ln)
			if len(fields) >= 2 {
				n, _ := strconv.Atoi(fields[1])
				count = n
			}
		}
	}
	return count, nil
}

// UIDFetchHeaders 拉取最近 maxCount 封的 UID 与信头（按序号窗口，避免 1:* 扫全箱）。
// total 为 Select 返回的 EXISTS；total<=0 时退化为 1:*。
func (c *IMAP) UIDFetchHeaders(total, maxCount int) ([]Summary, error) {
	if maxCount <= 0 {
		maxCount = 50
	}
	set := "1:*"
	if total > 0 {
		start := total - maxCount + 1
		if start < 1 {
			start = 1
		}
		set = fmt.Sprintf("%d:*", start)
	}
	resp, err := c.cmd("FETCH %s (UID BODY.PEEK[HEADER.FIELDS (FROM TO SUBJECT DATE MESSAGE-ID)])", set)
	if err != nil {
		return nil, err
	}
	if err := ok(resp); err != nil {
		return nil, err
	}
	return parseFetchSummaries(resp, maxCount), nil
}

// DebugFetch 调试用：返回原始响应。
func (c *IMAP) DebugFetch(total, maxCount int) (string, error) {
	set := "1:*"
	if total > 0 {
		start := total - maxCount + 1
		if start < 1 {
			start = 1
		}
		set = fmt.Sprintf("%d:*", start)
	}
	resp, err := c.cmd("FETCH %s (UID BODY.PEEK[HEADER.FIELDS (FROM TO SUBJECT DATE MESSAGE-ID)])", set)
	return resp, err
}

// UIDFetchBody 拉取一封全文（RFC822）。
func (c *IMAP) UIDFetchBody(uid uint32) ([]byte, error) {
	resp, err := c.cmdDeadline(60*time.Second, "UID FETCH %d (UID BODY.PEEK[])", uid)
	if err != nil {
		return nil, err
	}
	if err := ok(resp); err != nil {
		return nil, err
	}
	return extractLiteral(resp), nil
}

// Logout 退出。
func (c *IMAP) Logout() error {
	_, err := c.cmd("LOGOUT")
	c.conn.Close()
	return err
}

// Close 直接断开。
func (c *IMAP) Close() { c.conn.Close() }

func quoteIMAP(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

// parseFetchSummaries 从 FETCH 响应解析信头列表（按字节 literal，兼容 \r\n）。
func parseFetchSummaries(resp string, maxCount int) []Summary {
	var out []Summary
	// 形如: * <seq> FETCH (UID <uid> BODY[...] {n}\r\n<header>\r\n)
	i := 0
	for i < len(resp) {
		// 找 "FETCH (UID "
		j := strings.Index(resp[i:], "FETCH (UID ")
		if j < 0 {
			break
		}
		start := i + j
		// UID
		uidPart := resp[start+len("FETCH (UID "):]
		k := strings.IndexAny(uidPart, " \t\r\n")
		if k < 0 {
			break
		}
		uid, _ := strconv.ParseUint(uidPart[:k], 10, 32)
		// literal {n}
		lit := strings.Index(resp[start:], "{")
		if lit < 0 {
			i = start + 10
			continue
		}
		br := strings.Index(resp[start+lit:], "}")
		if br < 0 {
			break
		}
		n, err := strconv.Atoi(resp[start+lit+1 : start+lit+br])
		if err != nil {
			i = start + 10
			continue
		}
		// 跳过 }\r\n
		dataStart := start + lit + br + 1
		for dataStart < len(resp) && (resp[dataStart] == '\r' || resp[dataStart] == '\n') {
			dataStart++
		}
		dataEnd := dataStart + n
		if dataEnd > len(resp) {
			dataEnd = len(resp)
		}
		hdr := resp[dataStart:dataEnd]
		s := Summary{UID: uint32(uid), Date: time.Now()}
		s.From = decodeHeader(headerValue(hdr, "From"))
		s.To = decodeHeader(headerValue(hdr, "To"))
		s.Subject = decodeHeader(headerValue(hdr, "Subject"))
		if ds := headerValue(hdr, "Date"); ds != "" {
			if tm, err := mail.ParseDate(ds); err == nil {
				s.Date = tm
			}
		}
		out = append(out, s)
		if maxCount > 0 && len(out) >= maxCount {
			break
		}
		i = dataEnd
	}
	// 按日期倒序
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Date.After(out[i].Date) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// headerValue 从原始信头块取字段值（支持折行）。
func headerValue(raw, name string) string {
	prefix := strings.ToLower(name) + ":"
	for _, ln := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ln)), prefix) {
			return strings.TrimSpace(strings.TrimSpace(ln)[len(prefix):])
		}
	}
	return ""
}

// parseUID 从 FETCH 文本取 UID n。
func parseUID(s string) uint32 {
	fields := strings.Fields(s)
	for i, f := range fields {
		f = strings.Trim(f, "(,)")
		if strings.EqualFold(f, "UID") && i+1 < len(fields) {
			n, err := strconv.ParseUint(strings.Trim(fields[i+1], "(),\""), 10, 32)
			if err == nil {
				return uint32(n)
			}
		}
	}
	return 0
}

// extractLiteral 取响应中第一个 {n} literal 字节。
func extractLiteral(resp string) []byte {
	i := strings.Index(resp, "{")
	if i < 0 {
		return []byte(resp)
	}
	j := strings.Index(resp[i:], "}")
	if j < 0 {
		return []byte(resp)
	}
	n, err := strconv.Atoi(resp[i+1 : i+j])
	if err != nil || n < 0 {
		return []byte(resp)
	}
	start := i + j + 1
	// skip CRLF after }
	for start < len(resp) && (resp[start] == '\r' || resp[start] == '\n') {
		start++
	}
	end := start + n
	if end > len(resp) {
		end = len(resp)
	}
	return []byte(resp[start:end])
}

var _ = io.EOF
