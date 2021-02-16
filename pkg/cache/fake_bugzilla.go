package cache

import (
	"time"

	"github.com/eparis/bugzilla"
)

type FakeBugzillaClient struct {
	*bugzilla.Fake
}

func (f *FakeBugzillaClient) GetCachedExternalBugs(id int, lastChangedTime string) ([]bugzilla.ExternalBug, error) {
	return f.GetCachedExternalBugs(id, lastChangedTime)
}

func (f *FakeBugzillaClient) GetCachedBug(id int, lastChangedTime string) (*bugzilla.Bug, time.Duration, error) {
	b, err := f.GetBug(id)
	return b, 0, err
}

func (f *FakeBugzillaClient) GetCachedBugComments(id int, lastChangedTime string) ([]bugzilla.Comment, error) {
	return f.GetBugComments(id)
}

func (f *FakeBugzillaClient) GetCachedBugHistory(id int, lastChangedTime string) ([]bugzilla.History, error) {
	return f.GetBugHistory(id)
}
