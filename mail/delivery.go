package mail

import (
	"context"
	"database/sql"
	stdmail "net/mail"
	"strings"

	"tempmail/store"
)

type DeliveryService struct {
	store *store.Store
}

type DeliveryInput struct {
	Recipient string
	Sender    string
	Subject   string
	BodyText  string
	BodyHTML  string
	Raw       string
}

type DeliveryResult struct {
	Status string
}

func NewDeliveryService(s *store.Store) *DeliveryService {
	return &DeliveryService{store: s}
}

func NormalizeAddress(raw string) (string, string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", "", ErrInvalidRecipient
	}

	if addr, err := stdmail.ParseAddress(raw); err == nil {
		raw = strings.ToLower(strings.TrimSpace(addr.Address))
	} else {
		raw = strings.Trim(raw, "<>")
	}

	parts := strings.Split(raw, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrInvalidRecipient
	}

	return raw, parts[1], nil
}

func (d *DeliveryService) ValidateRecipient(ctx context.Context, recipient string) (string, error) {
	normalized, domain, err := NormalizeAddress(recipient)
	if err != nil {
		return "", err
	}
	if _, err := d.store.GetDomainByName(ctx, domain); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrInactiveDomain
		}
		return "", err
	}
	return normalized, nil
}

func (d *DeliveryService) Deliver(ctx context.Context, input DeliveryInput) (*DeliveryResult, error) {
	recipient, _, err := NormalizeAddress(input.Recipient)
	if err != nil {
		return nil, err
	}

	mailbox, err := d.store.GetMailboxByFullAddress(ctx, recipient)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		if _, retainErr := d.store.InsertRetainedMail(ctx, recipient, input.Sender, input.Subject, input.BodyText, input.BodyHTML, input.Raw); retainErr != nil {
			return nil, retainErr
		}
		return &DeliveryResult{Status: "retained"}, nil
	}

	if _, err := d.store.InsertEmail(ctx, mailbox.ID, input.Sender, input.Subject, input.BodyText, input.BodyHTML, input.Raw); err != nil {
		return nil, err
	}
	return &DeliveryResult{Status: "delivered"}, nil
}
