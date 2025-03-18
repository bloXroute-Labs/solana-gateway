package slackbot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/slack-go/slack"
)

type DialogHandler struct {
	slack  *Slack
	logger *log.Logger
}

func NewDialogHandler(slack *Slack, logger *log.Logger) *DialogHandler {
	return &DialogHandler{
		slack:  slack,
		logger: logger,
	}
}

func (h *DialogHandler) Start(ctx context.Context) error {
	msgChan, err := h.slack.ListenForMessages(ctx)
	if err != nil {
		return fmt.Errorf("failed to start message listener: %w", err)
	}

	go h.handleMessages(ctx, msgChan)
	return nil
}

func (h *DialogHandler) handleMessages(ctx context.Context, msgChan <-chan *slack.MessageEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}

			go h.processMessage(ctx, msg)
		}
	}
}

func (h *DialogHandler) processMessage(ctx context.Context, msg *slack.MessageEvent) {
	h.logger.Printf("Processing message: %s", msg.Text)

	// Split message text by commas and trim whitespace
	parts := strings.Split(msg.Text, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Sort the numbers
	sorted, err := h.sortInts(parts)
	if err != nil {
		h.logger.Printf("Error sorting numbers: %v", err)
		if err := h.slack.PostError(fmt.Sprintf("Error sorting numbers: %v", err)); err != nil {
			h.logger.Printf("Failed to post error message: %v", err)
		}
		return
	}

	// Format sorted array as string
	result := h.formatSortedNumbers(sorted)

	// Post the sorted result back to Slack
	if err := h.slack.PostMessage(result); err != nil {
		h.logger.Printf("Error posting response: %v", err)
		if err := h.slack.PostError(fmt.Sprintf("Error posting response: %v", err)); err != nil {
			h.logger.Printf("Failed to post error message: %v", err)
		}
	}
}

func (h *DialogHandler) sortInts(intsStrings []string) ([]int, error) {
	var ints = make([]int, len(intsStrings))
	for i, s := range intsStrings {
		var err error
		ints[i], err = strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("failed to convert '%s' to number: %w", s, err)
		}
	}
	sort.Ints(ints)
	return ints, nil
}

func (h *DialogHandler) formatSortedNumbers(numbers []int) string {
	var result strings.Builder
	result.WriteString("Sorted numbers: [")
	for i, num := range numbers {
		if i > 0 {
			result.WriteString(", ")
		}
		result.WriteString(strconv.Itoa(num))
	}
	result.WriteString("]")
	return result.String()
}
