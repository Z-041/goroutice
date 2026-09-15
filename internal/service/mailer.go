package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"time"

	"goroutice/internal/config"
)

// Mailer 抽象邮件发送，便于替换不同实现。
type Mailer interface {
	// SendVerificationEmail 发送邮箱验证邮件，token 为验证令牌。
	SendVerificationEmail(to, token string) error
	// SendPasswordResetEmail 发送密码重置邮件，token 为重置令牌。
	SendPasswordResetEmail(to, token string) error
}

// LogMailer 开发/测试用邮件实现，将验证信息写入日志而非真实发送。
type LogMailer struct{}

// NewLogMailer 构造 LogMailer。
func NewLogMailer() *LogMailer { return &LogMailer{} }

// SendVerificationEmail 记录验证邮件信息到日志。
func (m *LogMailer) SendVerificationEmail(to, token string) error {
	slog.Info("send verification email (log mailer)", "to", to, "token", token)
	return nil
}

// SendPasswordResetEmail 记录密码重置邮件信息到日志。
func (m *LogMailer) SendPasswordResetEmail(to, token string) error {
	slog.Info("send password reset email (log mailer)", "to", to, "token", token)
	return nil
}

// SmtpMailer 基于 net/smtp 的真实邮件发送实现，支持 STARTTLS 与隐式 TLS。
type SmtpMailer struct {
	host               string
	port               int
	username           string
	password           string
	from               string
	fromName           string
	useTLS             bool
	insecureSkipVerify bool
}

// NewSmtpMailer 构造 SmtpMailer。
func NewSmtpMailer(cfg config.SMTPConfig) *SmtpMailer {
	return &SmtpMailer{
		host:               cfg.Host,
		port:               cfg.Port,
		username:           cfg.Username,
		password:           cfg.Password,
		from:               cfg.From,
		fromName:           cfg.FromName,
		useTLS:             cfg.UseTLS,
		insecureSkipVerify: cfg.InsecureSkipVerify,
	}
}

// SendVerificationEmail 发送邮箱验证邮件。
func (m *SmtpMailer) SendVerificationEmail(to, token string) error {
	subject := "验证你的邮箱"
	body := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px;">
  <h2 style="margin:0 0 16px;">验证你的邮箱</h2>
  <p>你好，感谢注册。请使用以下验证令牌完成邮箱验证：</p>
  <p style="font-size:22px;font-weight:bold;letter-spacing:2px;background:#f4f4f5;padding:12px 16px;border-radius:6px;">%s</p>
  <p style="color:#71717a;font-size:13px;">该令牌有效期内可验证一次，请勿转发给他人。若非本人操作，请忽略此邮件。</p>
</div>`, html.EscapeString(token))
	return m.send(to, subject, body)
}

// SendPasswordResetEmail 发送密码重置邮件。
func (m *SmtpMailer) SendPasswordResetEmail(to, token string) error {
	subject := "重置你的密码"
	body := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px;">
  <h2 style="margin:0 0 16px;">重置你的密码</h2>
  <p>你好，我们收到了你的密码重置请求。请使用以下重置令牌设置新密码：</p>
  <p style="font-size:22px;font-weight:bold;letter-spacing:2px;background:#f4f4f5;padding:12px 16px;border-radius:6px;">%s</p>
  <p style="color:#71717a;font-size:13px;">该令牌有效期内可重置一次，请勿转发给他人。若非本人操作，请忽略此邮件。</p>
</div>`, html.EscapeString(token))
	return m.send(to, subject, body)
}

// send 构建并投递邮件。
func (m *SmtpMailer) send(to, subject, body string) error {
	addr := net.JoinHostPort(m.host, strconv.Itoa(m.port))
	msg := m.buildMessage(to, subject, body)

	if m.useTLS {
		return m.sendOverTLS(addr, to, msg)
	}

	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}
	return smtp.SendMail(addr, auth, m.from, []string{to}, msg)
}

// smtpDialTimeout 是建立 SMTP 连接的超时上限。
// 不设超时的话，SMTP 服务器无响应时连接会一直挂着，直到操作系统的 TCP 超时（可能长达数分钟）；
// 注册与重置密码接口会连带卡住，用户只能看到请求一直不返回。
const smtpDialTimeout = 10 * time.Second

// sendOverTLS 通过隐式 TLS（SMTPS，通常 465 端口）发送。
func (m *SmtpMailer) sendOverTLS(addr, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName:         m.host,
		InsecureSkipVerify: m.insecureSkipVerify, //nolint:gosec // 由配置控制，默认关闭，仅用于自建/测试 SMTP
	}
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: smtpDialTimeout},
		Config:    tlsConfig,
	}
	// 这里没有可用的 ctx（Mailer 接口不带 ctx），超时靠上面的 NetDialer 施加。
	conn, err := dialer.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if m.username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return err
		}
	}
	if err := client.Mail(m.from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// buildMessage 组装符合 MIME 规范的邮件原文。
func (m *SmtpMailer) buildMessage(to, subject, body string) []byte {
	var buf bytes.Buffer
	buf.WriteString("From: " + m.formatFrom() + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
	buf.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)
	return buf.Bytes()
}

// formatFrom 组装发件人，含显示名称时按 RFC 2047 编码。
func (m *SmtpMailer) formatFrom() string {
	if m.fromName == "" {
		return m.from
	}
	return mime.QEncoding.Encode("UTF-8", m.fromName) + " <" + m.from + ">"
}
