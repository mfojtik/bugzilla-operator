package controller

import (
	"context"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type Controller struct {
	newBugzillaClient             func(debug bool) cache.BugzillaClient
	slackClient, slackDebugClient slack.ChannelClient
}

func NewController(newBugzillaClient func(debug bool) cache.BugzillaClient, slackClient, slackDebugClient slack.ChannelClient) Controller {
	return Controller{
		newBugzillaClient, slackClient, slackDebugClient,
	}
}

func (c *Controller) NewBugzillaClient(ctx context.Context) cache.BugzillaClient {
	debug, ok := ctx.Value("debug").(bool)
	if ok && debug {
		return c.newBugzillaClient(true)
	}
	return c.newBugzillaClient(false)
}

func (c *Controller) SlackClient(ctx context.Context) slack.ChannelClient {
	debug, ok := ctx.Value("debug").(bool)
	if ok && debug {
		return c.slackDebugClient
	}
	return c.slackClient
}
