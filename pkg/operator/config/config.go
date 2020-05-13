package config

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
