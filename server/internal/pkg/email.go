package pkg

import(
	"fake_tiktok/internal/config"
	"net/smtp"
	"github.com/jordan-wright/email"
	"fmt"
	"crypto/tls"
	"strings"
)

func Email(To, subject string, body string, cfg *config.Config) error {
	to := strings.Split(To, ",") // 将收件人邮箱地址按逗号拆分成多个地址
	return SendEmail(to, subject, body, cfg)
}

func SendEmail(to []string, subject string, body string, cfg *config.Config) error {
	emailcfg := cfg.Email

	auth := smtp.PlainAuth("", emailcfg.From, emailcfg.Secret, emailcfg.Host)
  e := email.NewEmail()
	if emailcfg.Nickname != ""{
		e.From = fmt.Sprintf("%s <%s>", emailcfg.Nickname, emailcfg.From)
	}else{
		e.From = emailcfg.From
	}
	e.To = to
	e.Subject = subject
	e.HTML = []byte(body)

	var err error
	hostAdrr := fmt.Sprintf("%s:%d", emailcfg.Host, emailcfg.Port)
	if emailcfg.IsTLS{
		err = e.SendWithTLS(hostAdrr, auth, &tls.Config{ServerName: emailcfg.Host})
	}else{
		err = e.Send(hostAdrr, auth)
	}
	return err
}
