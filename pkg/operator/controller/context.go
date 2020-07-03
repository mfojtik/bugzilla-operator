package controller

import (
	"context"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type ControllerContext struct {
	newBugzillaClient             func(debug bool) cache.BugzillaClient
	slackClient, slackDebugClient slack.ChannelClient
}

func NewControllerContext(newBugzillaClient func(debug bool) cache.BugzillaClient, slackClient, slackDebugClient slack.ChannelClient) ControllerContext {
	return ControllerContext{
		newBugzillaClient, slackClient, slackDebugClient,
	}
}

func (c *ControllerContext) NewBugzillaClient(ctx context.Context) cache.BugzillaClient {
	debug, ok := ctx.Value("debug").(bool)
	if ok && debug {
		return c.newBugzillaClient(true)
	}
	return c.newBugzillaClient(false)
}

func (c *ControllerContext) SlackClient(ctx context.Context) slack.ChannelClient {
	debug, ok := ctx.Value("debug").(bool)
	if ok && debug {
		return c.slackDebugClient
	}
	return c.slackClient
}

func (c *ControllerContext) GetPersistentValue(key string) (string, error) {
	panic("implement")
	return "", nil
}

func (c *ControllerContext) SetPersistentValue(key, value string) error {
	panic("implement")
	return nil
}
