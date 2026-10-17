package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/store"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultBaseURL     = "https://api.telegram.org"
	defaultPollTimeout = 30
)

// Bot implements the M0 Telegram long-polling proof-of-concept adapter.
type Bot struct {
	Backend      app.API
	Store        *store.Store
	token        string
	allowedUsers map[int64]bool
	BaseURL      string
	Version      string
	PollTimeout  int
	Client       *http.Client
	offset       atomic.Int64
	acctOnce     sync.Once
	acct         string
	// seen holds the per-user UpdatedAt watermark of already queued terminal jobs; only the runDurable goroutine touches it.
	seen map[int64]int64
}

// New creates a Telegram Bot with an explicit token and allowlist of user IDs.
func New(token string, allowedUserIDs []int64, version string) (*Bot, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("telegram: bot token is required")
	}
	if len(allowedUserIDs) == 0 {
		return nil, errors.New("telegram: allowlist must contain at least one user ID")
	}
	allow := make(map[int64]bool, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		allow[id] = true
	}
	if version == "" {
		version = "0.1.0-alpha"
	}
	return &Bot{
		token:        token,
		allowedUsers: allow,
		BaseURL:      defaultBaseURL,
		Version:      version,
		PollTimeout:  defaultPollTimeout,
		Client:       &http.Client{Timeout: 45 * time.Second},
	}, nil
}

// NewFromEnv initializes a Bot from APW_TELEGRAM_TOKEN and APW_TELEGRAM_ALLOW.
func NewFromEnv(version string) (*Bot, error) {
	token := strings.TrimSpace(os.Getenv("APW_TELEGRAM_TOKEN"))
	if token == "" {
		return nil, errors.New("telegram: environment variable APW_TELEGRAM_TOKEN is not set")
	}
	rawAllow := strings.TrimSpace(os.Getenv("APW_TELEGRAM_ALLOW"))
	if rawAllow == "" {
		return nil, errors.New("telegram: environment variable APW_TELEGRAM_ALLOW is not set")
	}

	ids, err := ParseAllowlist(rawAllow)
	if err != nil {
		return nil, err
	}
	return New(token, ids, version)
}

// ParseAllowlist parses a comma-separated list of Telegram user IDs.
func ParseAllowlist(csv string) ([]int64, error) {
	var ids []int64
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("telegram: invalid user ID %q in allowlist: %w", part, err)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("telegram: allowlist is empty")
	}
	return ids, nil
}

type getUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []update `json:"result"`
}

type update struct {
	UpdateID int64    `json:"update_id"`
	Message  *message `json:"message,omitempty"`
}

type message struct {
	MessageID int64  `json:"message_id"`
	From      *user  `json:"from,omitempty"`
	Chat      chat   `json:"chat"`
	Text      string `json:"text"`
}

type user struct {
	ID int64 `json:"id"`
}

type chat struct {
	ID int64 `json:"id"`
}

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

// Offset returns the next update offset to be requested.
func (b *Bot) Offset() int64 {
	return b.offset.Load()
}

// Run starts the long-polling loop until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	if b.Backend != nil && b.Store != nil {
		return b.runDurable(ctx)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		updates, err := b.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}

		for _, u := range updates {
			if u.UpdateID >= b.offset.Load() {
				b.offset.Store(u.UpdateID + 1)
			}
			if u.Message == nil || u.Message.From == nil {
				continue
			}
			if !b.allowedUsers[u.Message.From.ID] {
				// Silently ignore messages from users not in allowlist
				continue
			}

			text := strings.TrimSpace(u.Message.Text)
			if text == "" {
				continue
			}

			reply := u.Message.Text
			if text == "/status" {
				reply = fmt.Sprintf("apw %s ok", b.Version)
			}

			_ = b.sendMessage(ctx, u.Message.Chat.ID, reply)
		}
	}
}

func (b *Bot) apiURL(method string) string {
	base := strings.TrimRight(b.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	return fmt.Sprintf("%s/bot%s/%s", base, b.token, method)
}

func (b *Bot) getUpdates(ctx context.Context) ([]update, error) {
	url := fmt.Sprintf("%s?timeout=%d&offset=%d", b.apiURL("getUpdates"), b.PollTimeout, b.offset.Load())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, b.sanitizeErr(err)
	}

	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, b.sanitizeErr(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, b.sanitizeErr(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram getUpdates status %d", resp.StatusCode)
	}

	var parsed getUpdatesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("telegram decode getUpdates: %w", err)
	}
	if !parsed.OK {
		return nil, errors.New("telegram getUpdates returned ok=false")
	}
	return parsed.Result, nil
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string) error {
	payload, err := json.Marshal(sendMessageRequest{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL("sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return b.sanitizeErr(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.Client.Do(req)
	if err != nil {
		return b.sanitizeErr(err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
	}()
	var result struct {
		OK         bool `json:"ok"`
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&result)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Parameters.RetryAfter > 0 {
			return &retryError{delay: time.Duration(result.Parameters.RetryAfter) * time.Second}
		}
		return fmt.Errorf("telegram sendMessage status %d", resp.StatusCode)
	}
	if decodeErr != nil || !result.OK {
		return errors.New("telegram sendMessage did not confirm delivery")
	}
	return nil
}

func (b *Bot) sanitizeErr(err error) error {
	if err == nil || b.token == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, b.token) {
		return err
	}
	return errors.New(strings.ReplaceAll(msg, b.token, "[REDACTED]"))
}
