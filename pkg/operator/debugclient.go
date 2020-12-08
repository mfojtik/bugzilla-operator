package operator

import (
	"fmt"

	"github.com/eparis/bugzilla"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type loggingReadOnlyClient struct {
	delegate           cache.BugzillaClient // intentionally not embedded to catch interface changes
	slackLoggingClient slack.ChannelClient
}

var _ cache.BugzillaClient = &loggingReadOnlyClient{}

func (lrc *loggingReadOnlyClient) GetCachedBug(id int, lastChangedTime string) (*bugzilla.Bug, error) {
	return lrc.delegate.GetCachedBug(id, lastChangedTime)
}

func (lrc *loggingReadOnlyClient) GetCachedBugComments(id int, lastChangedTime string) ([]bugzilla.Comment, error) {
	return lrc.delegate.GetCachedBugComments(id, lastChangedTime)
}

func (lrc *loggingReadOnlyClient) GetCachedBugHistory(id int, lastChangedTime string) ([]bugzilla.History, error) {
	return lrc.delegate.GetCachedBugHistory(id, lastChangedTime)
}

func (lrc *loggingReadOnlyClient) GetCachedExternalBugs(id int, lastChangedTime string) ([]bugzilla.ExternalBug, error) {
	return lrc.delegate.GetCachedExternalBugs(id, lastChangedTime)
}

func (lrc *loggingReadOnlyClient) Endpoint() string {
	return lrc.delegate.Endpoint()
}

func (lrc *loggingReadOnlyClient) GetBug(id int) (*bugzilla.Bug, error) {
	return lrc.delegate.GetBug(id)
}

func (lrc *loggingReadOnlyClient) GetBugComments(id int) ([]bugzilla.Comment, error) {
	return lrc.delegate.GetBugComments(id)
}

func (lrc *loggingReadOnlyClient) GetBugHistory(id int) ([]bugzilla.History, error) {
	return lrc.delegate.GetBugHistory(id)
}

func (lrc *loggingReadOnlyClient) Search(query bugzilla.Query) ([]*bugzilla.Bug, error) {
	return lrc.delegate.Search(query)
}

func (lrc *loggingReadOnlyClient) GetExternalBugs(id int) ([]bugzilla.ExternalBug, error) {
	return lrc.delegate.GetExternalBugs(id)
}

func (lrc *loggingReadOnlyClient) GetExternalBugPRsOnBug(id int) ([]bugzilla.ExternalBug, error) {
	return lrc.delegate.GetExternalBugPRsOnBug(id)
}

func (lrc *loggingReadOnlyClient) UpdateBug(id int, update bugzilla.BugUpdate) error {
	msg := fmt.Sprintf("Faking UpdateBug(%d, %#v)", id, update)
	klog.Info(msg)
	lrc.slackLoggingClient.MessageChannel(msg)
	return nil
}

func (lrc *loggingReadOnlyClient) WithCGIClient(user, password string) bugzilla.Client {
	return lrc.delegate.WithCGIClient(user, password)
}

func (lrc *loggingReadOnlyClient) BugList(queryName, sharerID string) ([]bugzilla.Bug, error) {
	return lrc.delegate.BugList(queryName, sharerID)
}

func (lrc *loggingReadOnlyClient) AddPullRequestAsExternalBug(id int, org, repo string, num int) (bool, error) {
	msg := fmt.Sprintf("Faking AddPullRequestAsExternalBug(%d, %q, %q, %d)", id, org, repo, num)
	klog.Info(msg)
	lrc.slackLoggingClient.MessageChannel(msg)
	return false, nil
}
