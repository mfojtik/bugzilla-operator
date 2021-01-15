package slack

import (
	"fmt"

	"github.com/slack-go/slack"
)

var peopleWithWrongSlackEmail = map[string]string{
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
}

type slackClient struct {
	client                *slack.Client
	channel, adminChannel string
	debug                 bool
}

func BugzillaToSlackEmail(originalEmail string) string {
	realEmail, ok := peopleWithWrongSlackEmail[originalEmail]
	if ok {
		return realEmail
	}
	return originalEmail
}

func SlackEmailToBugzilla(slackEmail string) string {
	for slack, bz := range peopleWithWrongSlackEmail {
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
	_, _, err := c.client.PostMessage(c.channel, slack.MsgOptionText(message, false))
	return err
}

func (c *slackClient) MessageAdminChannel(message string) error {
	if c.debug {
		message = fmt.Sprintf("DEBUG ADMIN #%s: %s", c.adminChannel, message)
	}
	_, _, err := c.client.PostMessage(c.adminChannel, slack.MsgOptionText(message, false))
	return err
}

func (c *slackClient) MessageEmail(email, message string) error {
	if c.debug {
		return c.MessageChannel(fmt.Sprintf("DEBUG: %q will receive:\n%s", email, message))
	}
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
	_, _, err = c.client.PostMessage(channel.ID, slack.MsgOptionText(message, false))
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
