// SMTP 发信：隐式 TLS(465) 或 STARTTLS(587)，纯文本正文。
package mail

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SendMail 发送一封纯文本邮件。
func SendMail(a *Account, to, subject, body string) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("收件人为空")
	}
	from := a.Email
	name := a.Name
	if name != "" {
		from = fmt.Sprintf("%s <%s>", name, a.Email)
	}
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + encodeHeader(subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: 8bit",
		"Date: " + time.Now().Format(time.RFC1123Z),
		"",
		strings.ReplaceAll(body, "\n", "\r\n"),
		"",
	}, "\r\n")

	addr := net.JoinHostPort(a.SMTPHost, strconv.Itoa(a.SMTPPort))
	auth := smtp.PlainAuth("", a.Email, a.Password, a.SMTPHost)

	if a.SMTPImplicitTLS() {
		return smtpSendTLS(addr, a.SMTPHost, auth, from, to, []byte(msg), a.SkipTLS)
	}
	return smtpSendStartTLS(addr, a.SMTPHost, auth, from, to, []byte(msg), a.SkipTLS)
}

func smtpSendTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte, skipVerify bool) error {
	dialer := net.Dialer{Timeout: 30 * time.Second}
	cfg := loadRootCAs(skipVerify)
	cfg.ServerName = host
	conn, err := tls.DialWithDialer(&dialer, "tcp", addr, cfg)
	if err != nil {
		return fmt.Errorf("SMTP 连接 %s: %w", addr, err)
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Quit()
	return smtpFinish(c, auth, from, to, msg, host)
}

func smtpSendStartTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte, skipVerify bool) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP 连接 %s: %w", addr, err)
	}
	defer c.Quit()
	if ok, _ := c.Extension("STARTTLS"); ok {
		cfg := loadRootCAs(skipVerify)
		cfg.ServerName = host
		if err := c.StartTLS(cfg); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	return smtpFinish(c, auth, from, to, msg, host)
}

func smtpFinish(c *smtp.Client, auth smtp.Auth, from, to string, msg []byte, host string) error {
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("SMTP 认证: %w", err)
			}
		}
	}
	if err := c.Mail(from); err != nil {
		// from 可能带显示名，退回裸地址
		addr := from
		if i := strings.Index(addr, "<"); i >= 0 {
			addr = addr[i+1:]
		}
		addr = strings.TrimSuffix(strings.TrimSpace(addr), ">")
		if err2 := c.Mail(addr); err2 != nil {
			return fmt.Errorf("SMTP MAIL: %w", err)
		}
		from = addr
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// encodeHeader 对 Subject 做 RFC2047 Base64（必要时）。
func encodeHeader(s string) string {
	ascii := true
	for _, r := range s {
		if r > 127 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	return mimeEncode(s)
}

func mimeEncode(s string) string {
	// 手写 RFC2047 =?UTF-8?B?...?=
	const chunk = 45
	var parts []string
	b := []byte(s)
	for i := 0; i < len(b); i += chunk {
		end := i + chunk
		if end > len(b) {
			end = len(b)
		}
		// 避免截断 UTF-8
		for end < len(b) && b[end]&0xC0 == 0x80 {
			end++
		}
		parts = append(parts, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString(b[i:end])+"?=")
	}
	return strings.Join(parts, " ")
}
