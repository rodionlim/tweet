package notifier

import (
	"context"
	"os"

	"github.com/rodionlim/tweet/library/log"
	"github.com/slack-go/slack"
)

// Notifier is a struct that has the responsibility of notifying users about an event.
type Notifier interface {
	Notify(msg string, args interface{}) error
}

// Slack is a notifier that notifies users via slack messages.
type Slack struct {
	client *slack.Client
}

// SlackArgs are the arguments to instantiate a Slack client.
type SlackArgs struct {
	ChannelID  string
	Attachment *slack.Attachment
}

func NewSlacker() *Slack {
	logger := log.Ctx(context.Background())
	token, exists := os.LookupEnv("SLACK_ACCESS_TOKEN")
	if !exists {
		logger.Error("Invalid slack access token. Please set \"SLACK_ACCESS_TOKEN\" variable")
		os.Exit(1)
	}

	return &Slack{
		client: slack.New(token),
	}
}

// Notify sends slack messages.
// It requires the env SLACK_ACCESS_TOKEN to be set.
func (s *Slack) Notify(msg string, args interface{}) error {
	logger := log.Ctx(context.Background())
	msgOptions := []slack.MsgOption{
		slack.MsgOptionText(msg, false),
		slack.MsgOptionAsUser(true), // Add this if you want that the bot would post message as a user, otherwise it will send response using the default slackbot
	}
	slackArgs := args.(SlackArgs)
	if slackArgs.Attachment != nil {
		msgOptions = append(msgOptions, slack.MsgOptionAttachments(*slackArgs.Attachment))
	}
	channelID, timestamp, err := s.client.PostMessage(
		slackArgs.ChannelID,
		msgOptions...,
	)
	if err != nil {
		logger.Errorf("%s slack args %+v\n", err, slackArgs)
		return err
	}
	logger.Infof("Message successfully sent to channel %s at %s\n", channelID, timestamp)
	return nil
}
