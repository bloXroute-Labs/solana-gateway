package slackbot

import (
	"context"
	"fmt"
	"net/http"

	"github.com/slack-go/slack"
)

// Config contains configuration for Slack bot
type Config struct {
	AppToken  string
	ChannelID string
}

type Slack struct {
	Config *Config
	Client *slack.Client
}

func New(cfg *Config, client *http.Client) (*Slack, error) {
	c := slack.New(cfg.AppToken, slack.OptionHTTPClient(client))
	_, err := c.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("bot auth failure: %w", err)
	}

	return &Slack{
		Config: cfg,
		Client: c,
	}, nil
}

const (
	errorEmoji   = "❌"
	successEmoji = "✅"
	warningEmoji = "⚠️"
)

func (s *Slack) PostError(m string, append ...string) error {
	return s.PostMessage(fmt.Sprintf("%s %s", errorEmoji, m), append...)
}

func (s *Slack) PostSuccess(m string, append ...string) error {
	return s.PostMessage(fmt.Sprintf("%s %s", successEmoji, m), append...)
}

func (s *Slack) PostWarning(m string, append ...string) error {
	return s.PostMessage(fmt.Sprintf("%s %s", warningEmoji, m), append...)
}

func (s *Slack) PostMessage(m string, append ...string) error {
	for _, a := range append {
		m += fmt.Sprintf("\n%s", a)
	}

	_, _, err := s.Client.PostMessage(s.Config.ChannelID, slack.MsgOptionText(m, false))
	return err
}

func (s *Slack) PostMessageWithAttachment(m string, attachment slack.Attachment) error {
	_, _, err := s.Client.PostMessage(s.Config.ChannelID, slack.MsgOptionText(m, false), slack.MsgOptionAttachments(attachment))
	return err
}

func (s *Slack) ListenForMessages(ctx context.Context) (<-chan *slack.MessageEvent, error) {
	rtm := s.Client.NewRTM()
	msgChan := make(chan *slack.MessageEvent)

	go func() {
		defer close(msgChan)
		go rtm.ManageConnection()

		for {
			select {
			case <-ctx.Done():
				rtm.Disconnect()
				return
			case msg := <-rtm.IncomingEvents:
				switch ev := msg.Data.(type) {
				case *slack.MessageEvent:
					// Only forward messages from the configured channel
					if ev.Channel == s.Config.ChannelID {
						msgChan <- ev
					}
				case *slack.RTMError:
					s.PostError(fmt.Sprintf("Slack RTM error: %v", ev))
				case *slack.InvalidAuthEvent:
					s.PostError("Invalid slack credentials")
					return
				}
			}
		}
	}()

	return msgChan, nil
}
