package bugutil

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type stagingBugzillaClient struct {
	originalClient bugzilla.Client
	slackClient    slack.ChannelClient
}

func NewStagingBugzillaClient(c bugzilla.Client, s slack.ChannelClient) bugzilla.Client {
	return &stagingBugzillaClient{
		originalClient: c,
		slackClient:    s,
	}
}

func (s *stagingBugzillaClient) Endpoint() string {
	return s.originalClient.Endpoint()
}

func (s *stagingBugzillaClient) GetBug(id int) (*bugzilla.Bug, error) {
	return s.originalClient.GetBug(id)
}

func (s *stagingBugzillaClient) Search(query bugzilla.Query) ([]*bugzilla.Bug, error) {
	return s.originalClient.Search(query)
}

func (s *stagingBugzillaClient) GetExternalBugPRsOnBug(id int) ([]bugzilla.ExternalBug, error) {
	return s.originalClient.GetExternalBugPRsOnBug(id)
}

func (s *stagingBugzillaClient) UpdateBug(id int, update bugzilla.BugUpdate) error {
	s.slackClient.MessageChannel(fmt.Sprintf("Bug %d with:\n```\n%s\n```\n", id, spew.Sdump(update)))
	return nil
}

func (s *stagingBugzillaClient) AddPullRequestAsExternalBug(id int, org, repo string, num int) (bool, error) {
	return s.originalClient.AddPullRequestAsExternalBug(id, org, repo, num)
}

func (s *stagingBugzillaClient) WithCGIClient(user, password string) bugzilla.Client {
	return s.originalClient.WithCGIClient(user, password)
}

func (s *stagingBugzillaClient) BugList(queryName, sharerID string) ([]bugzilla.Bug, error) {
	return s.originalClient.BugList(queryName, sharerID)
}
