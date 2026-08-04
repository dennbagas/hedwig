package slackbot

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
)

type slackClient struct {
	api *slack.Client
}

func New(botToken string) Client {
	return &slackClient{api: slack.New(botToken)}
}

func (c *slackClient) PostMessage(ctx context.Context, channel, text string, buttons []Button) (string, error) {
	opts := []slack.MsgOption{slack.MsgOptionText(text, false)}
	if len(buttons) > 0 {
		opts = append(opts, slack.MsgOptionBlocks(buildBlocks(text, buttons)...))
	}
	_, ts, err := c.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		return "", fmt.Errorf("post message: %w", err)
	}
	return ts, nil
}

func (c *slackClient) UpdateMessage(ctx context.Context, channel, ts, text string, buttons []Button) error {
	opts := []slack.MsgOption{slack.MsgOptionText(text, false)}
	if len(buttons) > 0 {
		opts = append(opts, slack.MsgOptionBlocks(buildBlocks(text, buttons)...))
	} else {
		// MsgOptionBlocks with no arguments sends blocks: [], explicitly
		// clearing any buttons from the previous version of this message.
		opts = append(opts, slack.MsgOptionBlocks())
	}
	_, _, _, err := c.api.UpdateMessageContext(ctx, channel, ts, opts...)
	if err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	return nil
}

func buildBlocks(text string, buttons []Button) []slack.Block {
	sectionText := slack.NewTextBlockObject(slack.MarkdownType, text, false, false)
	section := slack.NewSectionBlock(sectionText, nil, nil)

	elements := make([]slack.BlockElement, len(buttons))
	for i, b := range buttons {
		elements[i] = slack.NewButtonBlockElement("", b.Value, slack.NewTextBlockObject(slack.PlainTextType, b.Text, false, false))
	}
	actions := slack.NewActionBlock("", elements...)

	return []slack.Block{section, actions}
}
