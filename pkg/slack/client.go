package slack

import (
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
	MessageEmail(email, message string) error
}

type slackClient struct {
	client  *slack.Client
	channel string
}

func getEmail(originalEmail string) string {
	realEmail, ok := peopleWithWrongSlackEmail[originalEmail]
	if ok {
		return realEmail
	}
	return originalEmail
}

func (c *slackClient) MessageChannel(message string) error {
	_, _, err := c.client.PostMessage(c.channel, slack.MsgOptionText(message, false))
	return err
}

func (c *slackClient) MessageEmail(email, message string) error {
	user, err := c.client.GetUserByEmail(getEmail(email))
	if err != nil {
		return err
	}
	_, _, chanID, err := c.client.OpenIMChannel(user.ID)
	if err != nil {
		return err
	}
	_, _, err = c.client.PostMessage(chanID, slack.MsgOptionText(message, false))
	return err
}

func NewChannelClient(client *slack.Client, channel string) ChannelClient {
	c := &slackClient{
		channel: channel,
		client:  client,
	}
	return c
}
