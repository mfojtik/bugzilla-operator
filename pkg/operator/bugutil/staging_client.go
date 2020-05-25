package bugutil

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type stagingBugzillaClient struct {
	bugzilla.Client
	slackClient slack.ChannelClient
}

func NewStagingBugzillaClient(c bugzilla.Client, s slack.ChannelClient) bugzilla.Client {
	return &stagingBugzillaClient{
		Client:      c,
		slackClient: s,
	}
}

func (s *stagingBugzillaClient) UpdateBug(id int, update bugzilla.BugUpdate) error {
	s.slackClient.MessageChannel(fmt.Sprintf("Bug %d with:\n```\n%s\n```\n", id, spew.Sdump(update)))
	return nil
}
