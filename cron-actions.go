package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mileusna/crontab"
	"github.com/reeveci/reeve-lib/schema"
)

func NewCronActions(plugin *GiteaPlugin) *CronActions {
	a := &CronActions{
		plugin: plugin,
		repos:  make(map[string]cronActionSet),
	}
	return a
}

type CronActions struct {
	lock   sync.Mutex
	plugin *GiteaPlugin
	repos  map[string]cronActionSet
}

func (a *CronActions) UpdateRules(repository string, rules CronRuleset) {
	a.lock.Lock()
	defer a.lock.Unlock()

	prev, found := a.repos[repository]
	if found {
		if prev.rules.compare(rules) {
			return
		}

		prev.handler.Clear()
		prev.handler.Shutdown()
	}

	if len(rules) == 0 {
		if found {
			a.plugin.Log.Info(fmt.Sprintf("clearing cron triggers for repository %s", repository))
			delete(a.repos, repository)
		}
		return
	}

	a.plugin.Log.Info(fmt.Sprintf("updating cron triggers for repository %s", repository))

	handler := crontab.New()
	a.repos[repository] = cronActionSet{handler, rules}

	for cron, actions := range rules {
		actionNames := make([]string, 0, len(actions))
		messages := make([]schema.Message, 0, len(actions))
		for action, enabled := range actions {
			if enabled {
				actionNames = append(actionNames, string(action))
				messages = append(messages, schema.Message{
					Target: PLUGIN_NAME,
					Options: map[string]string{
						"type":   "action",
						"action": string(action),
					},
				})
			}
		}
		if len(messages) > 0 {
			err := handler.AddJob(string(cron), a.sendMessages, fmt.Sprintf("triggering scheduled cron actions - %s", strings.Join(actionNames, ", ")), messages)
			if err != nil {
				a.plugin.Log.Error(fmt.Sprintf("error registering Cron job - %s", err))
			}
		}
	}
}

func (a *CronActions) Close() {
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, set := range a.repos {
		set.handler.Clear()
		set.handler.Shutdown()
	}

	a.repos = nil
}

func (a *CronActions) sendMessages(logMessage string, messages []schema.Message) {
	a.plugin.Log.Info(logMessage)
	err := a.plugin.API.NotifyMessages(messages)
	if err != nil {
		a.plugin.Log.Error(fmt.Sprintf("error sending Cron actions - %s", err))
	}
}

type cronActionSet struct {
	handler *crontab.Crontab
	rules   CronRuleset
}

func NewCronRuleset() CronRuleset {
	return make(CronRuleset)
}

type CronRuleset map[Cron]map[ActionName]bool

type Cron string
type ActionName string

func (r CronRuleset) Add(cronExpression, action string) {
	if cronExpression == "" || action == "" {
		return
	}
	cron := Cron(cronExpression)
	actionName := ActionName(action)
	if actions, found := r[cron]; found {
		actions[actionName] = true
	} else {
		r[cron] = map[ActionName]bool{(actionName): true}
	}
}

func (r CronRuleset) compare(other CronRuleset) bool {
	if len(r) != len(other) {
		return false
	}
	for cron, actions := range r {
		otherActions, found := other[cron]
		if !found || len(actions) != len(otherActions) {
			return false
		}
		for action, enabled := range actions {
			if otherEnabled, found := otherActions[action]; !found || enabled != otherEnabled {
				return false
			}
		}
	}
	return true
}
