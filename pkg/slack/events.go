package slack

import "github.com/slack-go/slack/slackevents"

type EventBus interface {
	SubscribeLinkShared(func(ev *slackevents.LinkSharedEvent)) error
}
