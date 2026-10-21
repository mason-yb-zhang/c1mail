// 账号配置：IMAP/SMTP 连接参数，持久化在 data/account.json。
package mail

import (
	"encoding/json"
	"fmt"
	"os"
)

// Account 邮箱账号配置。密码为应用专用密码或授权码。
type Account struct {
	Email     string `json:"email"`
	IMAPHost  string `json:"imapHost"`
	IMAPPort  int    `json:"imapPort"`
	SMTPHost  string `json:"smtpHost"`
	SMTPPort  int    `json:"smtpPort"`
	Password  string `json:"password"`
	Name      string `json:"name,omitempty"`
	SSL       bool   `json:"ssl"`       // 旧字段：同时当 IMAP/SMTP 隐式 TLS
	IMAPSSL   bool   `json:"imapSsl"`   // true: IMAP 隐式 TLS(993)
	SMTPSSL   bool   `json:"smtpSsl"`   // true: SMTP 隐式 TLS(465)；false: STARTTLS(587)
	SkipTLS   bool   `json:"skipTls"`   // 跳过证书校验（不推荐，仅测试）
}

// IMAPImplicitTLS 按端口推断 IMAP 是否隐式 TLS（993 是，143 否）。
func (a *Account) IMAPImplicitTLS() bool {
	if a.IMAPPort == 993 {
		return true
	}
	if a.IMAPPort == 143 {
		return false
	}
	return a.IMAPSSL || a.SSL
}

// SMTPImplicitTLS 按端口推断 SMTP 是否隐式 TLS（465 是，587 否=STARTTLS）。
func (a *Account) SMTPImplicitTLS() bool {
	if a.SMTPPort == 465 {
		return true
	}
	if a.SMTPPort == 587 {
		return false
	}
	return a.SMTPSSL || a.SSL
}

func (a *Account) Validate() error {
	if a.Email == "" || a.IMAPHost == "" || a.SMTPHost == "" {
		return fmt.Errorf("邮箱/IMAP/SMTP 主机必填")
	}
	if a.IMAPPort <= 0 {
		a.IMAPPort = 993
	}
	if a.SMTPPort <= 0 {
		// 默认 587 STARTTLS；465 隐式 TLS 在配置里显式写
		a.SMTPPort = 587
	}
	return nil
}

// LoadAccount 读账号配置；文件缺失返回 (nil, nil)。
func LoadAccount(path string) (*Account, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a Account
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("解析账号: %w", err)
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return &a, nil
}

// SaveAccount 原子写账号配置。
func SaveAccount(path string, a *Account) error {
	if err := a.Validate(); err != nil {
		return err
	}
	return atomicWrite(path, a)
}
