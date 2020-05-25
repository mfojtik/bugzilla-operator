package cache

import (
	"encoding/json"
	"fmt"

	"github.com/eparis/bugzilla"
	"k8s.io/klog"
)

type cachedClient struct {
	bugzilla.Client
}

type BugzillaClient interface {
	bugzilla.Client

	GetCachedBug(id int, lastChangedTime string) (*bugzilla.Bug, error)
	GetCachedBugComments(id int, lastChangedTime string) ([]bugzilla.Comment, error)
	GetCachedBugHistory(id int, lastChangedTime string) ([]bugzilla.History, error)
}

func NewCachedBugzillaClient(client bugzilla.Client) BugzillaClient {
	return &cachedClient{client}
}

func (c *cachedClient) GetBug(id int) (*bugzilla.Bug, error) {
	b, err := c.Client.GetBug(id)
	if err != nil {
		return nil, err
	}
	bs, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	Set(fmt.Sprintf("bug-%d", b.ID), b.LastChangeTime, bs)
	return b, nil
}

func (c *cachedClient) GetCachedBug(id int, lastChangedTime string) (*bugzilla.Bug, error) {
	bs, err := Get(fmt.Sprintf("bug-%d", id), lastChangedTime)
	if err != nil {
		klog.Warningf("failed to get cached bug %d: %v", id, err)
	}
	if bs != nil {
		ret := bugzilla.Bug{}
		if err := json.Unmarshal(bs, &ret); err != nil {
			klog.Warningf("failed to decode cached bug %d: %v", id, err)
		} else {
			return &ret, nil
		}
	}
	return c.GetBug(id)
}

func (c *cachedClient) GetCachedBugComments(id int, lastChangedTime string) ([]bugzilla.Comment, error) {
	key := fmt.Sprintf("comments-%d", id)
	bs, err := Get(key, lastChangedTime)
	if err != nil {
		klog.Warningf("failed to get cached comments for bug %d: %v", id, err)
	}
	if bs != nil {
		ret := []bugzilla.Comment{}
		if err := json.Unmarshal(bs, &ret); err != nil {
			klog.Warningf("failed to decode cached comments for bug %d: %v", id, err)
		} else {
			return ret, nil
		}
	}
	ret, err := c.GetBugComments(id)
	if err != nil {
		return nil, err
	}
	bs, err = json.Marshal(ret)
	if err != nil {
		klog.Warningf("failed to encode cached comments for bug %d: %v", id, err)
	} else {
		Set(key, lastChangedTime, bs)
	}
	return ret, nil
}

func (c *cachedClient) GetCachedBugHistory(id int, lastChangedTime string) ([]bugzilla.History, error) {
	key := fmt.Sprintf("history-%d", id)
	bs, err := Get(key, lastChangedTime)
	if err != nil {
		klog.Warningf("failed to get cached history for bug %d: %v", id, err)
	}
	if bs != nil {
		ret := []bugzilla.History{}
		if err := json.Unmarshal(bs, &ret); err != nil {
			klog.Warningf("failed to decode cached history for bug %d: %v", id, err)
		} else {
			return ret, nil
		}
	}
	ret, err := c.GetBugHistory(id)
	if err != nil {
		return nil, err
	}
	bs, err = json.Marshal(ret)
	if err != nil {
		klog.Warningf("failed to encode cached history for bug %d: %v", id, err)
	} else {
		Set(key, lastChangedTime, bs)
	}
	return ret, nil
}
