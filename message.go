package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/reeveci/reeve-lib/schema"
)

func (p *GiteaPlugin) Message(source string, message schema.Message) error {
	switch {
	case source == schema.MESSAGE_SOURCE_SERVER:
		if message.Options["event"] == schema.EVENT_STARTUP_COMPLETE {
			p.Log.Info("triggering initial discovery scan")
			p.Scanner.Scan()
		}
		return nil

	default:
		if source == "webui" && message.Options["webui"] == "present" {
			p.Lock()
			p.WebUIPresent = true
			p.Unlock()

			SendActionBundleMessage(p, ActionBundle{
				BundleID: "operations",
				Actions: []Action{
					{
						ID:     "rescan",
						Name:   "rescan repositories",
						Groups: []string{"gitea"},
						Message: schema.Message{
							Target: PLUGIN_NAME,
							Options: map[string]string{
								"type":      "operation",
								"operation": "rescan",
							},
						},
					},
				},
			})

			return nil
		}
	}

	switch message.Options["type"] {
	case "webhook":
		var webhook Webhook
		err := json.Unmarshal(message.Data, &webhook)
		if err != nil {
			return fmt.Errorf("error parsing webhook message %s", message.Data)
		}

		p.Scanner.Notify(webhook.Repository.FullName)

		commitMessage := strings.ToLower(webhook.HeadCommit.Message)
		if strings.Contains(commitMessage, "[skip ci]") || strings.Contains(commitMessage, "[ci skip]") {
			return nil
		}

		trigger := map[string]string{
			"type":          "git",
			"trigger":       "push",
			"ref":           webhook.Ref,
			"commit":        webhook.HeadCommit.ID,
			"commitMessage": webhook.HeadCommit.Message,
			"repository":    webhook.Repository.FullName,
			"repositoryURL": webhook.Repository.HtmlURL,
			"cloneURL":      webhook.Repository.CloneURL,
			"defaultBranch": webhook.Repository.DefaultBranch,
			"files":         strings.Join(collectFiles(webhook), "\n"),
		}

		err = p.API.NotifyTriggers([]schema.Trigger{trigger})
		if err != nil {
			return fmt.Errorf("error notifying trigger - %s", err)
		}

	case "operation":
		operation := message.Options["operation"]
		if operation == "" {
			return fmt.Errorf("missing operation")
		}

		switch operation {
		case "rescan":
			p.Log.Info("triggering user requested discovery scan")
			p.Scanner.Scan()
		}

	case "action":
		action := message.Options["action"]
		if action == "" {
			return fmt.Errorf("missing action")
		}

		searchResult, err := p.Scanner.Search(message.Options["search"])
		if err != nil {
			return err
		}

		triggers := make([]schema.Trigger, 0, len(searchResult))

		for _, repo := range searchResult {
			commitResponse, err := p.Scanner.FetchCommit(repo.FullName, repo.DefaultBranch)
			if err != nil {
				return err
			}

			if commitResponse != nil {
				triggers = append(triggers, map[string]string{
					"type":          "git",
					"trigger":       "action",
					"action":        action,
					"ref":           fmt.Sprintf("refs/heads/%s", repo.DefaultBranch),
					"commit":        commitResponse.Commit.ID,
					"commitMessage": commitResponse.Commit.Message,
					"repository":    repo.FullName,
					"repositoryURL": repo.HtmlURL,
					"cloneURL":      repo.CloneURL,
					"defaultBranch": repo.DefaultBranch,
				})
			}
		}

		if len(triggers) == 0 {
			return nil
		}

		err = p.API.NotifyTriggers(triggers)
		if err != nil {
			return fmt.Errorf("error notifying triggers - %s", err)
		}
	}

	return nil
}
