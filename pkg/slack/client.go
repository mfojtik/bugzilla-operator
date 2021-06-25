package slack

import (
	"fmt"

	"github.com/slack-go/slack"
)

var bugzillaToSlackOverrides = map[string]string{
	"sttts@redhat.com":       "sschiman@redhat.com",
	"rphillips@redhat.com":   "rphillip@redhat.com",
	"adam.kaplan@redhat.com": "adkaplan@redhat.com",
	"wking@redhat.com":       "trking@redhat.com",
	"sanchezl@redhat.com":    "lusanche@redhat.com",
}

type ChannelClient interface {
	MessageChannel(message string) error
	MessageAdminChannel(message string) error
	MessageEmail(email, message string) error

	PostMessageChannel(options ...slack.MsgOption) error
	PostMessageAdminChannel(options ...slack.MsgOption) error
	PostMessageEmail(email string, options ...slack.MsgOption) error
}

type slackClient struct {
	client                *slack.Client
	channel, adminChannel string
	debug                 bool
}

func BugzillaToSlackEmail(bzEmail string) string {
	if realEmail, ok := bugzillaToSlackOverrides[bzEmail]; ok {
		return realEmail
	}
	return bzEmail
}

func SlackEmailToBugzilla(slackEmail string) string {
	for bz, slack := range bugzillaToSlackOverrides {
		if slackEmail == slack {
			return bz
		}
	}
	return slackEmail
}

func (c *slackClient) MessageChannel(message string) error {
	if c.debug {
		message = fmt.Sprintf("DEBUG CHANNEL #%s: %s", c.channel, message)
	}
	return c.PostMessageChannel(slack.MsgOptionText(message, false))
}

func (c *slackClient) MessageAdminChannel(message string) error {
	if c.debug {
		message = fmt.Sprintf("DEBUG ADMIN #%s: %s", c.adminChannel, message)
	}
	return c.PostMessageAdminChannel(slack.MsgOptionText(message, false))
}

func (c *slackClient) MessageEmail(email, message string) error {
	if c.debug {
		return c.MessageChannel(fmt.Sprintf("DEBUG: %q will receive:\n%s", email, message))
	}
	return c.PostMessageEmail(email, slack.MsgOptionText(message, false))
}

func (c *slackClient) PostMessageChannel(options ...slack.MsgOption) error {
	_, _, err := c.client.PostMessage(c.channel, options...)
	return err
}

func (c *slackClient) PostMessageAdminChannel(options ...slack.MsgOption) error {
	_, _, err := c.client.PostMessage(c.adminChannel, options...)
	return err
}

func (c *slackClient) PostMessageEmail(email string, options ...slack.MsgOption) error {
	user, err := c.client.GetUserByEmail(BugzillaToSlackEmail(email))
	if err != nil {
		return err
	}
	channel, _, _, err := c.client.OpenConversation(&slack.OpenConversationParameters{
		ReturnIM: true,
		Users:    []string{user.ID},
	})
	if err != nil {
		return err
	}
	_, _, err = c.client.PostMessage(channel.ID, options...)
	return err
}

func NewChannelClient(client *slack.Client, channel, adminChannel string, debug bool) ChannelClient {
	c := &slackClient{
		channel:      channel,
		adminChannel: adminChannel,
		client:       client,
		debug:        debug,
	}
	return c
}
