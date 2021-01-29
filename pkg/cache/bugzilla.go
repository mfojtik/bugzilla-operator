package cache

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"k8s.io/klog"
)

type cachedClient struct {
	bugzilla.Client
	cachePrefix string
}

type BugzillaClient interface {
	bugzilla.Client

	GetCachedBug(id int, lastChangedTime string) (*bugzilla.Bug, time.Duration, error)
	GetCachedBugComments(id int, lastChangedTime string) ([]bugzilla.Comment, error)
	GetCachedBugHistory(id int, lastChangedTime string) ([]bugzilla.History, error)
	GetCachedExternalBugs(id int, lastChangedTime string) ([]bugzilla.ExternalBug, error)
}

type Option func(c *cachedClient)

func NewCachedBugzillaClient(client bugzilla.Client, opts ...Option) BugzillaClient {
	c := &cachedClient{client, ""}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func CustomCachePrefix(cachePrefix string) Option {
	return func(c *cachedClient) {
		if cachePrefix != "" && !strings.HasSuffix(cachePrefix, "-") {
			cachePrefix = cachePrefix + "-"
		}
		c.cachePrefix = cachePrefix
	}
}

type cachedBug struct {
	*bugzilla.Bug

	CacheTime string `json:"cache_time,omitempty"`
}

func (c *cachedClient) GetBug(id int) (*bugzilla.Bug, error) {
	now := time.Now()
	b, err := c.Client.GetBug(id)
	if err != nil {
		return nil, err
	}
	cb := cachedBug{b, now.Format(time.RFC3339)}
	bs, err := json.Marshal(&cb)
	if err != nil {
		return nil, err
	}
	Set(fmt.Sprintf("%sbug-%d", c.cachePrefix, b.ID), b.LastChangeTime, bs)
	return b, nil
}

func (c *cachedClient) GetCachedBug(id int, lastChangedTime string) (*bugzilla.Bug, time.Duration, error) {
	bs, err := Get(fmt.Sprintf("%sbug-%d", c.cachePrefix, id), lastChangedTime)
	if err != nil {
		klog.Warningf("failed to get cached bug %d: %v", id, err)
	}
	if bs != nil {
		cb := cachedBug{}
		if err := json.Unmarshal(bs, &cb); err != nil {
			klog.Warningf("failed to decode cached bug %d: %v", id, err)
		} else {
			lastTimeVerifiedString := cb.CacheTime
			if cb.CacheTime == "" {
				lastTimeVerifiedString = cb.LastChangeTime
			}

			lastTimeVerified, err := time.Parse(time.RFC3339, lastTimeVerifiedString)
			if err != nil {
				klog.Warningf("invalid lastTimeVerifiedString %q for bug %d: %v", lastTimeVerifiedString, id, err)
			} else {
				return cb.Bug, time.Now().Sub(lastTimeVerified), nil
			}
		}
	}
	b, err := c.GetBug(id)
	return b, 0, err
}

func (c *cachedClient) GetCachedExternalBugs(id int, lastChangedTime string) ([]bugzilla.ExternalBug, error) {
	b, _, err := c.GetCachedBug(id, lastChangedTime)
	if err != nil {
		return nil, err
	}
	bs, err := Get(fmt.Sprintf("%sexternal-bugs-%d", c.cachePrefix, id), b.LastChangeTime)
	if err != nil {
		klog.Warningf("failed to get cached external bugs %d: %v", id, err)
	}
	if bs != nil {
		ret := bugzilla.Bug{}
		if err := json.Unmarshal(bs, &ret); err != nil {
			klog.Warningf("failed to decode cached external bugs %d: %v", id, err)
		} else {
			return ret.ExternalBugs, nil
		}
	}

	ret, err := c.GetExternalBugs(id)
	if err != nil {
		return nil, err
	}

	bs, err = json.Marshal(bugzilla.Bug{ExternalBugs: ret})
	if err != nil {
		return nil, err
	}
	Set(fmt.Sprintf("%sexternal-bugs-%d", c.cachePrefix, b.ID), b.LastChangeTime, bs)

	return ret, nil
}

func (c *cachedClient) GetCachedBugComments(id int, lastChangedTime string) ([]bugzilla.Comment, error) {
	key := fmt.Sprintf("%scomments-%d", c.cachePrefix, id)
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
	key := fmt.Sprintf("%shistory-%d", c.cachePrefix, id)
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
