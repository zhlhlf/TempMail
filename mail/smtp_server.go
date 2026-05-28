package mail

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	smtp "github.com/emersion/go-smtp"
)

type SMTPConfig struct {
	Addr            string
	Domain          string
	MaxMessageBytes int64
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	MaxRecipients   int
}

type SMTPServer struct {
	server *smtp.Server
}

func NewSMTPServer(cfg SMTPConfig, delivery *DeliveryService) *SMTPServer {
	backend := &smtpBackend{delivery: delivery}
	server := smtp.NewServer(backend)
	server.Addr = cfg.Addr
	server.Domain = cfg.Domain
	server.MaxRecipients = cfg.MaxRecipients
	server.MaxMessageBytes = cfg.MaxMessageBytes
	server.AllowInsecureAuth = false
	server.ReadTimeout = cfg.ReadTimeout
	server.WriteTimeout = cfg.WriteTimeout
	server.ErrorLog = smtpLogger{}
	return &SMTPServer{server: server}
}

func (s *SMTPServer) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func (s *SMTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

type smtpBackend struct {
	delivery *DeliveryService
}

func (b *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{delivery: b.delivery}, nil
}

type smtpSession struct {
	delivery   *DeliveryService
	sender     string
	recipients []string
}

func (s *smtpSession) Reset() {
	s.sender = ""
	s.recipients = nil
}

func (s *smtpSession) Logout() error {
	s.Reset()
	return nil
}

func (s *smtpSession) Mail(from string, _ *smtp.MailOptions) error {
	s.sender = strings.TrimSpace(from)
	s.recipients = nil
	log.Printf("[smtp] MAIL FROM=<%s>", s.sender)
	return nil
}

func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	normalized, err := s.delivery.ValidateRecipient(context.Background(), to)
	if err != nil {
		if err == ErrInvalidRecipient {
			log.Printf("[smtp] RCPT TO=<%s> rejected: invalid recipient address", strings.TrimSpace(to))
			return &smtp.SMTPError{Code: 550, Message: "invalid recipient address"}
		}
		if err == ErrInactiveDomain {
			log.Printf("[smtp] RCPT TO=<%s> rejected: recipient domain is not active", strings.TrimSpace(to))
			return &smtp.SMTPError{Code: 550, Message: "recipient domain is not active"}
		}
		log.Printf("[smtp] RCPT TO=<%s> temporary failure: %v", strings.TrimSpace(to), err)
		return &smtp.SMTPError{Code: 451, Message: "temporary validation failure"}
	}
	s.recipients = append(s.recipients, normalized)
	log.Printf("[smtp] RCPT TO=<%s> accepted", normalized)
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return &smtp.SMTPError{Code: 451, Message: "failed to read message body"}
	}

	parsed, parseErr := ParseMessage(raw)
	if parseErr != nil {
		log.Printf("[smtp] MIME parse warning: %v", parseErr)
		parsed = &ParsedMessage{Raw: string(raw)}
	}
	if parsed.Sender == "" {
		parsed.Sender = s.sender
	}
	log.Printf("[smtp] DATA accepted from=<%s> recipients=%s bytes=%d subject=%q", parsed.Sender, strings.Join(s.recipients, ","), len(raw), parsed.Subject)

	for _, recipient := range s.recipients {
		result, err := s.delivery.Deliver(context.Background(), DeliveryInput{
			Recipient: recipient,
			Sender:    parsed.Sender,
			Subject:   parsed.Subject,
			BodyText:  parsed.BodyText,
			BodyHTML:  parsed.BodyHTML,
			Raw:       parsed.Raw,
		})
		if err != nil {
			if err == ErrUnknownMailbox {
				log.Printf("[smtp] delivery rejected recipient=<%s> from=<%s>: mailbox not found", recipient, parsed.Sender)
				return &smtp.SMTPError{Code: 550, Message: "mailbox not found"}
			}
			log.Printf("[smtp] delivery failed recipient=<%s> from=<%s>: %v", recipient, parsed.Sender, err)
			return &smtp.SMTPError{Code: 451, Message: fmt.Sprintf("delivery failed: %v", err)}
		}
		log.Printf("[smtp] message stored recipient=<%s> status=%s from=<%s> subject=%q", recipient, result.Status, parsed.Sender, parsed.Subject)
	}
	return nil
}

type smtpLogger struct{}

func (smtpLogger) Println(args ...any) {
	prefixed := append([]any{"[smtp]"}, args...)
	log.Println(prefixed...)
}

func (smtpLogger) Printf(format string, args ...any) {
	log.Printf("[smtp] "+format, args...)
}
