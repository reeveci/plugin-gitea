package main

import (
	"net/http"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/reeveci/reeve-lib/plugin"
	"github.com/reeveci/reeve-lib/schema"
)

const PLUGIN_NAME = "gitea"

func main() {
	log := hclog.New(&hclog.LoggerOptions{})

	plugin.Serve(&plugin.PluginConfig{
		Plugin: &GiteaPlugin{
			Log: log,

			http: &http.Client{},
		},

		Logger: log,
	})
}

type GiteaPlugin struct {
	InternalUrl, PublicUrl, CloneUrl string
	Token                            string
	Unrestricted                     bool
	TaskDomains                      map[string]string
	TrustedDomains, TrustedTasks     []string
	SetupTask                        string
	SecretKey                        string
	DiscoverySchedule                string

	Log hclog.Logger
	API plugin.ReeveAPI

	WebUIPresent bool
	sync.Mutex

	CronActions *CronActions
	Scanner     *Scanner

	http *http.Client
}

func (p *GiteaPlugin) Name() (string, error) {
	return PLUGIN_NAME, nil
}

func (p *GiteaPlugin) Register(settings map[string]string, api plugin.ReeveAPI) (capabilities plugin.Capabilities, err error) {
	p.API = api
	p.CronActions = NewCronActions(p)

	var enabled bool
	if enabled, err = boolSetting(settings, "ENABLED"); !enabled || err != nil {
		return
	}
	if p.InternalUrl, err = requireSetting(settings, "URL"); err != nil {
		return
	}
	if p.InternalUrl, err = parseBaseURL(p.InternalUrl); err != nil {
		return
	}
	if p.PublicUrl, err = parseBaseURL(defaultSetting(settings, "PUBLIC_URL", p.InternalUrl)); err != nil {
		return
	}
	if p.CloneUrl, err = parseBaseURL(defaultSetting(settings, "CLONE_URL", p.InternalUrl)); err != nil {
		return
	}
	if p.Token, err = requireSetting(settings, "TOKEN"); err != nil {
		return
	}
	if p.Unrestricted, err = boolSetting(settings, "UNRESTRICTED"); err != nil {
		return
	}
	taskDomains := strings.Fields(settings["TASK_DOMAINS"])
	if domainCount := len(taskDomains); domainCount > 0 {
		p.TaskDomains = make(map[string]string, domainCount)
		for _, domain := range taskDomains {
			parts := strings.SplitN(domain, ":", 2)
			if len(parts) == 1 {
				p.TaskDomains[parts[0]] = ""
			} else {
				p.TaskDomains[parts[0]] = parts[1]
			}
		}
	}
	p.TrustedDomains = strings.Fields(settings["TRUSTED_DOMAINS"])
	p.TrustedTasks = strings.Fields(settings["TRUSTED_TASKS"])
	if p.SetupTask, err = requireSetting(settings, "SETUP_GIT_TASK"); err != nil {
		return
	}
	if p.SecretKey, err = requireSetting(settings, "SECRET_KEY"); err != nil {
		return
	}
	p.DiscoverySchedule = defaultSetting(settings, "DISCOVERY_SCHEDULE", "0 12 * * *")

	if p.Scanner, err = NewScanner(p); err != nil {
		return
	}

	capabilities.Message = true
	capabilities.Discover = true
	capabilities.CLIMethods = CLIMethods
	return
}

func (p *GiteaPlugin) Unregister() error {
	if p.Scanner != nil {
		p.Scanner.Close()
	}

	p.CronActions.Close()
	p.API.Close()

	return nil
}

func (p *GiteaPlugin) Resolve(env []string) (map[string]schema.Env, error) {
	return nil, nil
}

func (p *GiteaPlugin) Notify(status schema.PipelineStatus) error {
	return nil
}
