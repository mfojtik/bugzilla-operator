package config

import (
	"encoding/base64"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

type Credentials struct {
	Username               string `yaml:"username"`
	Password               string `yaml:"password"`
	APIKey                 string `yaml:"apiKey"`
	SlackToken             string `yaml:"slackToken"`
	SlackVerificationToken string `yaml:"slackVerificationToken"`
}

type Transition struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type BugzillaRelease struct {
	CurrentTargetRelease string   `yaml:"currentTargetRelease"`
	TargetReleases       []string `yaml:"targetReleases"`
}

type Group []string

type OperatorConfig struct {
	Credentials Credentials `yaml:"credentials"`

	StaleBugComment      string `yaml:"staleBugComment"`
	StaleBugCloseComment string `yaml:"staleBugCloseComment"`

	Release    BugzillaRelease `yaml:"release"`
	Components []string        `yaml:"components"`

	Groups map[string]Group `yaml:"groups"`

	// SlackChannel is a channel where the operator will post reports/etc.
	SlackChannel      string `yaml:"slackChannel"`
	SlackAdminChannel string `yaml:"slackAdminChannel"`

	CachePath string `yaml:"cachePath"`
}

// Anonymize makes a shallow copy of the config, suitable for dumping in logs (no sensitive data)
func (c *OperatorConfig) Anonymize() OperatorConfig {
	a := *c
	if user := a.Credentials.Username; len(user) > 0 {
		a.Credentials.Username = strings.Repeat("x", len(a.Credentials.DecodedUsername()))
	}
	if password := a.Credentials.Password; len(password) > 0 {
		a.Credentials.Password = strings.Repeat("x", len(a.Credentials.DecodedPassword()))
	}
	if key := a.Credentials.APIKey; len(key) > 0 {
		a.Credentials.APIKey = strings.Repeat("x", len(a.Credentials.DecodedAPIKey()))
	}
	if key := a.Credentials.SlackToken; len(key) > 0 {
		a.Credentials.SlackToken = strings.Repeat("x", len(a.Credentials.DecodedSlackToken()))
	}
	if key := a.Credentials.SlackVerificationToken; len(key) > 0 {
		a.Credentials.SlackVerificationToken = strings.Repeat("x", len(a.Credentials.DecodedSlackVerificationToken()))
	}
	return a
}

func decode(s string) string {
	if strings.HasPrefix(s, "base64:") {
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "base64:"))
		if err != nil {
			return s
		}
		return string(data)
	}
	return s
}

// DecodedAPIKey return decoded APIKey (in case it was base64 encoded)
func (b Credentials) DecodedAPIKey() string {
	return decode(b.APIKey)
}

// DecodedAPIKey return decoded Password (in case it was base64 encoded)
func (b Credentials) DecodedPassword() string {
	return decode(b.Password)
}

// DecodedAPIKey return decoded Username (in case it was base64 encoded)
func (b Credentials) DecodedUsername() string {
	return decode(b.Username)
}

func (b Credentials) DecodedSlackToken() string {
	return decode(b.SlackToken)
}

func (b Credentials) DecodedSlackVerificationToken() string {
	return decode(b.SlackVerificationToken)
}

func ExpandGroups(cfg map[string]Group, roots ...string) sets.String {
	users := sets.String{}
	for _, r := range roots {
		users, _ = expandGroup(cfg, r, users, nil)
	}
	return users
}

func expandGroup(cfg map[string]Group, x string, expanded sets.String, seen sets.String) (sets.String, sets.String) {
	if strings.HasPrefix(x, "group:") {
		group := x[6:]
		if seen.Has(group) {
			return expanded, seen
		}
		if seen == nil {
			seen = sets.String{}
		}
		seen = seen.Insert(group)
		for _, y := range cfg[group] {
			expanded, seen = expandGroup(cfg, y, expanded, seen)
		}
		return expanded, seen
	}

	return expanded.Insert(x), seen
}
