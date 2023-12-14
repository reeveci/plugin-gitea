package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/reeveci/reeve-lib/schema"
)

func NewWebUIScanner(plugin *GiteaPlugin, repository string) DocumentScanner {
	return &WebUIScanner{
		plugin:     plugin,
		repository: repository,
		actions:    make(map[string]bool),
	}
}

type WebUIScanner struct {
	plugin     *GiteaPlugin
	repository string
	actions    map[string]bool
	done       bool
}

func (s *WebUIScanner) Scan(document Document) error {
	switch document.Type {
	case "pipeline":
		for _, action := range document.When["action"].Include {
			s.actions[action] = true
		}

		for _, step := range document.Steps {
			for _, action := range step.When["action"].Include {
				s.actions[action] = true
			}
		}
	}

	return nil
}

func (s *WebUIScanner) Done() {
	s.done = true
}

func (s *WebUIScanner) Close() {
	bundle := ActionBundle{
		BundleID: "repo:" + s.repository,
	}

	if s.done {
		for action := range s.actions {
			if strings.HasPrefix(action, ":") {
				continue
			}

			parts := strings.Split(strings.Trim(action, ":"), ":")
			groups := append([]string{"pipeline triggers"}, parts[:len(parts)-1]...)
			name := parts[len(parts)-1]

			bundle.Actions = append(bundle.Actions, Action{
				ID:     "trigger:" + action,
				Name:   name,
				Groups: groups,
				Message: schema.Message{
					Target: PLUGIN_NAME,
					Options: map[string]string{
						"type":   "action",
						"action": action,
					},
				},
			})
		}
	}

	SendActionBundleMessage(s.plugin, bundle)
}

type ActionBundle struct {
	BundleID string   `json:"bundleID"`
	Actions  []Action `json:"actions"`
}

type Action struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Groups  []string       `json:"groups"`
	Message schema.Message `json:"message"`
}

func SendActionBundleMessage(plugin *GiteaPlugin, bundle ActionBundle) bool {
	data, err := json.Marshal(bundle)
	if err != nil {
		plugin.Log.Error(fmt.Sprintf("error building action bundle for WebUI - %s", err))
		return false
	}

	err = plugin.API.NotifyMessages([]schema.Message{{
		Target:  "webui",
		Options: map[string]string{"webui": "actions"},
		Data:    data,
	}})
	if err != nil {
		plugin.Log.Error(fmt.Sprintf("error sending actions to WebUI - %s", err))
		return false
	}

	return true
}
