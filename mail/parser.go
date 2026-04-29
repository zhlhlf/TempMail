package mail

import (
	"bytes"
	"io"
	"strings"

	message "github.com/emersion/go-message"
	msgmail "github.com/emersion/go-message/mail"
)

type ParsedMessage struct {
	Sender   string
	Subject  string
	BodyText string
	BodyHTML string
	Raw      string
}

func ParseMessage(raw []byte) (*ParsedMessage, error) {
	reader, err := msgmail.CreateReader(bytes.NewReader(raw))
	parsed := &ParsedMessage{Raw: string(raw)}
	if err != nil && reader == nil {
		return parsed, err
	}

	if reader == nil {
		return parsed, nil
	}

	if subject, subjectErr := reader.Header.Subject(); subjectErr == nil {
		parsed.Subject = subject
	}
	parsed.Sender = reader.Header.Get("From")

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return parsed, nil
		}
		if err != nil {
			return parsed, err
		}

		switch h := part.Header.(type) {
		case *msgmail.InlineHeader:
			contentType, _, ctErr := h.ContentType()
			if ctErr != nil {
				contentType = "text/plain"
			}
			body, readErr := io.ReadAll(part.Body)
			if readErr != nil {
				return parsed, readErr
			}
			text := string(body)
			switch {
			case contentType == "text/html" && parsed.BodyHTML == "":
				parsed.BodyHTML = text
			case contentType == "text/plain" && parsed.BodyText == "":
				parsed.BodyText = text
			case strings.HasPrefix(contentType, "text/") && parsed.BodyText == "":
				parsed.BodyText = text
			}
		}
	}
}

func isRecoverableMessageError(err error) bool {
	return message.IsUnknownCharset(err)
}
