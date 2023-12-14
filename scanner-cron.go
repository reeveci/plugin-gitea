package main

func NewCronScanner(plugin *GiteaPlugin, repository string) DocumentScanner {
	return &CronScanner{
		plugin:     plugin,
		repository: repository,
		rules:      NewCronRuleset(),
	}
}

type CronScanner struct {
	plugin     *GiteaPlugin
	repository string
	rules      CronRuleset
	done       bool
}

func (s *CronScanner) Scan(document Document) error {
	switch document.Type {
	case "trigger":
		if document.Cron != "" && document.Action != "" {
			s.rules.Add(document.Cron, document.Action)
		}
	}

	return nil
}

func (s *CronScanner) Done() {
	s.done = true
}

func (s *CronScanner) Close() {
	if !s.done {
		s.rules = nil
	}

	s.plugin.CronActions.UpdateRules(s.repository, s.rules)
}
