package config

func ApplyAutomaticReportsDefaults(r *AutomaticReport, allComponents []string) {
	if r.When == nil {
		r.When = []string{
			"CRON_TZ=Europe/Prague 30 9 1-7,16-23 * 2-4",
			"CRON_TZ=America/New_York 30 9 1-7,16-23 * 2-4",
		}
	}

	if r.Components == nil {
		r.Components = allComponents
	}

	if r.Reports == nil {
		r.Reports = []string{"blocker-bugs", "closed-bugs"}
	}
}

func ApplyDefaults(cfg *OperatorConfig) {
	if cfg.Schedules == nil && len(cfg.SlackChannel) > 0 {
		cfg.Schedules = append(cfg.Schedules,
			AutomaticReport{
				SlackChannel: cfg.SlackChannel,
			},
		)
	}

	for i := range cfg.Schedules {
		ApplyAutomaticReportsDefaults(&cfg.Schedules[i], cfg.Components.List())
	}
}
