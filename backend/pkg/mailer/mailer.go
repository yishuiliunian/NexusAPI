// Package mailer 提供简易 SMTP 发信封装。
//
// 使用标准库 net/smtp，不引入外部依赖。支持 PLAIN / CRAM-MD5 认证，
// 连接 STARTTLS（587 端口）或隐式 TLS（465 端口）由 AddrTLS 决定。
//
// 发信是同步的；调用方若需异步，自行用 go routine 或放进 asynq 队列。
package mailer

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// Config SMTP 配置。
type Config struct {
	Host     string // smtp.example.com
	Port     int    // 587 / 465 / 25
	Username string
	Password string
	From     string // From 地址（必填）
	UseTLS   bool   // 隐式 TLS（端口 465）
}

// Mailer 发信客户端。Addr() 返回 host:port。
type Mailer struct {
	cfg Config
}

// New 构造。空配置视作"未启用"—— Send 将返回 ErrDisabled，
// 调用方可据此 fallback 为日志输出。
func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// ErrDisabled 未配置 SMTP 时返回；上层可忽略。
var ErrDisabled = errors.New("mailer: not configured")

// Enabled 返回是否已配置 SMTP（Host + From 必填）。
func (m *Mailer) Enabled() bool {
	return m.cfg.Host != "" && m.cfg.From != ""
}

// Addr SMTP 服务器地址 host:port。
func (m *Mailer) Addr() string {
	port := m.cfg.Port
	if port == 0 {
		if m.cfg.UseTLS {
			port = 465
		} else {
			port = 587
		}
	}
	return fmt.Sprintf("%s:%d", m.cfg.Host, port)
}

// Send 发送一封 HTML 邮件。to 可多地址。
//
// subject 建议英文或 UTF-8（本函数按 RFC 2047 做 Base64 编码）。
// body 按 text/html; charset=UTF-8 发送。
func (m *Mailer) Send(to []string, subject, htmlBody string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	if len(to) == 0 {
		return errors.New("mailer: empty recipients")
	}
	msg := buildMessage(m.cfg.From, to, subject, htmlBody)
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	addr := m.Addr()

	if m.cfg.UseTLS {
		return sendTLS(addr, m.cfg.Host, auth, m.cfg.From, to, msg)
	}
	// STARTTLS (587) 或 plain (25)；net/smtp.SendMail 自己判断
	return smtp.SendMail(addr, auth, m.cfg.From, to, msg)
}

// buildMessage 组装 MIME 报文。
func buildMessage(from string, to []string, subject, htmlBody string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: =?UTF-8?B?%s?=\r\n", base64Encode(subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}

func base64Encode(s string) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	in := []byte(s)
	var out strings.Builder
	for i := 0; i < len(in); i += 3 {
		var b1, b2, b3 byte
		b1 = in[i]
		if i+1 < len(in) {
			b2 = in[i+1]
		}
		if i+2 < len(in) {
			b3 = in[i+2]
		}
		out.WriteByte(chars[b1>>2])
		out.WriteByte(chars[((b1&0x03)<<4)|(b2>>4)])
		if i+1 < len(in) {
			out.WriteByte(chars[((b2&0x0f)<<2)|(b3>>6)])
		} else {
			out.WriteByte('=')
		}
		if i+2 < len(in) {
			out.WriteByte(chars[b3&0x3f])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
}

// sendTLS 隐式 TLS（465 端口）发送。
func sendTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("mailer: dial tls: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("mailer: new client: %w", err)
	}
	defer client.Quit()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, t := range to {
		if err := client.Rcpt(t); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

// _ 避免未使用 net 包的 goimports 警告
var _ = net.Dial
