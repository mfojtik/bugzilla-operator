package slacker

import (
	"fmt"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

const (
	errorFormat = "*Error:* _%s_"
)

// A ResponseWriter interface is used to respond to an event
type ResponseWriter interface {
	Reply(text string, options ...ReplyOption) error
	ReportError(err error, options ...ReportErrorOption)
	Client() *slack.Client
}

// NewResponse creates a new response structure
func NewResponse(event *slackevents.MessageEvent, client *slack.Client) ResponseWriter {
	return &response{event: event, client: client}
}

type response struct {
	event  *slackevents.MessageEvent
	client *slack.Client
}

// ReportError sends back a formatted error message to the channel where we received the event from
func (r *response) ReportError(err error, options ...ReportErrorOption) {
	defaults := newReportErrorDefaults(options...)

	opts := []slack.MsgOption{
		slack.MsgOptionText(fmt.Sprintf(errorFormat, err.Error()), false),
	}
	if defaults.ThreadResponse {
		opts = append(opts, slack.MsgOptionTS(r.event.ThreadTimeStamp))
	}
	r.client.SendMessage(r.event.Channel, opts...)
}

// Reply send a attachments to the current channel with a message
func (r *response) Reply(message string, options ...ReplyOption) error {
	defaults := newReplyDefaults(options...)

	if defaults.ThreadResponse {
		_, _, err := r.client.PostMessage(
			r.event.Channel,
			slack.MsgOptionText(message, false),
			slack.MsgOptionAsUser(true),
			slack.MsgOptionAttachments(defaults.Attachments...),
			slack.MsgOptionBlocks(defaults.Blocks...),
			slack.MsgOptionTS(r.event.ThreadTimeStamp), // TODO: is this EventTimeStamp?
		)
		return err
	}

	_, _, err := r.client.PostMessage(
		r.event.Channel,
		slack.MsgOptionText(message, false),
		slack.MsgOptionAsUser(true),
		slack.MsgOptionAttachments(defaults.Attachments...),
		slack.MsgOptionBlocks(defaults.Blocks...),
	)
	return err
}

// ChannelClient returns the slack client
func (r *response) Client() *slack.Client {
	return r.client
}
