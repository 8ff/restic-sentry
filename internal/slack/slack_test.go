package slack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendSuccess(t *testing.T) {
	var received Message

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.NotifySuccess("Test Title", "Test details")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(received.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(received.Attachments))
	}
	att := received.Attachments[0]
	if att.Color != ColorSuccess {
		t.Errorf("color = %q, want %q", att.Color, ColorSuccess)
	}
	if att.Title != "Test Title" {
		t.Errorf("title = %q, want %q", att.Title, "Test Title")
	}
	if att.Footer != "restic-sentry" {
		t.Errorf("footer = %q, want %q", att.Footer, "restic-sentry")
	}
}

func TestSendServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.NotifySuccess("Test", "details")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestNotifyErrorColor(t *testing.T) {
	var received Message

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.NotifyError("Failure", "something broke")

	if received.Attachments[0].Color != ColorError {
		t.Errorf("expected error color %q, got %q", ColorError, received.Attachments[0].Color)
	}
}

func TestNotifyWarningColor(t *testing.T) {
	var received Message

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.NotifyWarning("Partial", "some files skipped")

	if received.Attachments[0].Color != ColorWarning {
		t.Errorf("expected warning color %q, got %q", ColorWarning, received.Attachments[0].Color)
	}
}
