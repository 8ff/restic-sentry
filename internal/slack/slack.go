package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	WebhookURL string
	HTTPClient *http.Client
}

func NewClient(webhookURL string) *Client {
	return &Client{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type Attachment struct {
	Color  string `json:"color"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	Footer string `json:"footer"`
	Ts     int64  `json:"ts"`
}

type Message struct {
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

const (
	ColorSuccess = "#36a64f" // green
	ColorWarning = "#ff9900" // orange
	ColorError   = "#cc0000" // red
)

func (c *Client) Send(msg *Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling slack message: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending slack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}
	return nil
}

// NotifySuccess sends a green success notification.
func (c *Client) NotifySuccess(title, details string) error {
	return c.Send(&Message{
		Attachments: []Attachment{{
			Color:  ColorSuccess,
			Title:  title,
			Text:   details,
			Footer: "restic-sentry",
			Ts:     time.Now().Unix(),
		}},
	})
}

// NotifyWarning sends an orange warning notification (e.g. partial backup).
func (c *Client) NotifyWarning(title, details string) error {
	return c.Send(&Message{
		Attachments: []Attachment{{
			Color:  ColorWarning,
			Title:  title,
			Text:   details,
			Footer: "restic-sentry",
			Ts:     time.Now().Unix(),
		}},
	})
}

// NotifyError sends a red error notification.
func (c *Client) NotifyError(title, details string) error {
	return c.Send(&Message{
		Attachments: []Attachment{{
			Color:  ColorError,
			Title:  title,
			Text:   details,
			Footer: "restic-sentry",
			Ts:     time.Now().Unix(),
		}},
	})
}
