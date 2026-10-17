package telegram

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/store"
)

type retryError struct{ delay time.Duration }

func (e *retryError) Error() string { return "telegram delivery rate limited" }
func (b *Bot) accountID() string {
	b.acctOnce.Do(func() {
		h := sha256.Sum256([]byte(b.token))
		b.acct = hex.EncodeToString(h[:16])
	})
	return b.acct
}
func telegramOwner(id int64) string { return "telegram:" + strconv.FormatInt(id, 10) }
func shorten(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "\n[truncated]"
	}
	return text
}

func (b *Bot) processUpdate(ctx context.Context, u update) error {
	account := b.accountID()
	done, err := b.Store.InboxDone(ctx, account, u.UpdateID)
	if err != nil || done {
		return err
	}
	var reply string
	var chatID int64
	if u.Message != nil && u.Message.From != nil {
		m := u.Message
		chatID = m.Chat.ID
		// Bind this workspace to allowlisted private users; group forwards cannot impersonate an owner.
		if b.allowedUsers[m.From.ID] && m.Chat.ID == m.From.ID {
			reply = b.command(ctx, telegramOwner(m.From.ID), account, u.UpdateID, m.Text)
		}
	}
	return b.Store.CompleteInbox(ctx, account, u.UpdateID, chatID, shorten(reply, 3900))
}

func (b *Bot) command(ctx context.Context, owner, account string, updateID int64, text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ""
	}
	command := strings.ToLower(strings.Split(fields[0], "@")[0])
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0]))
	var job *store.Job
	var err error
	switch command {
	case "/run", "/execute", "/review", "/preview":
		mode := core.ModeExecute
		if command == "/review" {
			mode = core.ModeReview
		}
		if command == "/preview" {
			mode = core.ModePreview
		}
		job, err = b.Backend.Submit(ctx, owner, app.Request{RequestID: fmt.Sprintf("%s:%d", account, updateID), Instruction: rest, Mode: mode})
	case "/status":
		if rest == "" {
			jobs, e := b.Backend.List(ctx, owner)
			if e != nil {
				return "Cannot read tasks"
			}
			if len(jobs) == 0 {
				return "No tasks in this workspace"
			}
			var lines []string
			for i, j := range jobs {
				if i >= 10 {
					break
				}
				lines = append(lines, j.ID+" — "+string(j.Status))
			}
			return strings.Join(lines, "\n")
		}
		job, err = b.Backend.Get(ctx, rest, owner)
	case "/report":
		job, err = b.Backend.Get(ctx, rest, owner)
		if err == nil {
			return taskText(job)
		}
	case "/cancel":
		err = b.Backend.Cancel(ctx, rest, owner)
		if err == nil {
			return "Cancellation requested for " + rest
		}
	case "/resume":
		job, err = b.Backend.Resume(ctx, rest, owner)
	case "/apply":
		job, err = b.Backend.Apply(ctx, rest, owner)
	default:
		return "Commands: /review <task>, /preview <task>, /run <task>, /status [task-id], /report <task-id>, /cancel <task-id>, /resume <task-id>, /apply <task-id>"
	}
	if err != nil {
		return "Task request failed: " + b.sanitizeErr(err).Error()
	}
	return job.ID + " — " + string(job.Status)
}

func taskText(job *store.Job) string {
	text := job.ID + " — " + string(job.Status)
	if job.Error != "" {
		text += "\n" + job.Error
	}
	var report executor.ExecutionReport
	if len(job.Report) > 0 && json.Unmarshal(job.Report, &report) == nil {
		text += "\n" + report.Summary
	}
	return shorten(text, 3900)
}

func (b *Bot) flush(ctx context.Context) error {
	account := b.accountID()
	if b.seen == nil {
		b.seen = make(map[int64]int64, len(b.allowedUsers))
	}
	for userID := range b.allowedUsers {
		jobs, err := b.Store.TerminalJobsSince(ctx, telegramOwner(userID), b.seen[userID])
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			continue
		}
		items := make([]store.Delivery, len(jobs))
		keys := make([]string, len(jobs))
		for i := range jobs {
			j := &jobs[i]
			keys[i] = fmt.Sprintf("task:%s:%s:%d", j.ID, j.Status, j.UpdatedAt.UnixNano())
			items[i] = store.Delivery{ChatID: userID, Text: taskText(j)}
		}
		if err := b.Store.QueueDeliveries(ctx, account, items, keys); err != nil {
			return err
		}
		b.seen[userID] = jobs[len(jobs)-1].UpdatedAt.UnixNano()
	}
	deliveries, err := b.Store.Deliveries(ctx, account)
	if err != nil {
		return err
	}
	for _, d := range deliveries {
		if !b.allowedUsers[d.ChatID] {
			continue
		}
		err := b.sendMessage(ctx, d.ChatID, d.Text)
		delay := time.Second << min(d.Attempts+1, 6)
		var limited *retryError
		if errors.As(err, &limited) {
			delay = limited.delay
		}
		if saveErr := b.Store.DeliveryResult(ctx, d.ID, err == nil, delay); saveErr != nil {
			return saveErr
		}
	}
	return nil
}

func (b *Bot) runDurable(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	offset, err := b.Store.TelegramOffset(ctx, b.accountID())
	if err != nil {
		return err
	}
	b.offset.Store(offset)
	type pollResult struct {
		updates []update
		err     error
	}
	results := make(chan pollResult)
	next := make(chan int64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			updates, err := b.getUpdates(ctx)
			select {
			case results <- pollResult{updates, err}:
			case <-ctx.Done():
				return
			}
			select {
			case offset := <-next:
				b.offset.Store(offset)
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() { cancel(); <-done }()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := b.flush(ctx); err != nil && ctx.Err() == nil {
				return err
			}
		case result := <-results:
			if result.err != nil {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(500 * time.Millisecond):
				}
			} else {
				for _, u := range result.updates {
					if err := b.processUpdate(ctx, u); err != nil {
						return err
					}
				}
				if err := b.flush(ctx); err != nil {
					return err
				}
			}
			offset, err := b.Store.TelegramOffset(ctx, b.accountID())
			if err != nil {
				return err
			}
			select {
			case next <- offset:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
