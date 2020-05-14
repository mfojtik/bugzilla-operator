package config

import (
	"encoding/base64"
	"strings"
)

type BugzillaCredentials struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	APIKey   string `yaml:"apiKey"`
}

type BugzillaLists struct {
	StaleListName string `yaml:"staleListName"`
	SharerID      string `yaml:"sharerID"`
}

type OperatorConfig struct {
	Credentials       BugzillaCredentials `yaml:"credentials"`
	Lists             BugzillaLists       `yaml:"lists"`
	StaleBugComment   string              `yaml:"staleBugComment"`
	DevWhiteboardFlag string              `yaml:"devWhiteboardFlag"`
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

func (b BugzillaCredentials) DecodedAPIKey() string {
	return decode(b.APIKey)
}

func (b BugzillaCredentials) DecodedPassword() string {
	return decode(b.Password)
}

func (b BugzillaCredentials) DecodedUsername() string {
	return decode(b.Username)
}
