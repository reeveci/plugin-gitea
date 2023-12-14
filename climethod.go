package main

import (
	"fmt"
	"strings"

	"github.com/reeveci/plugin-gitea/encryption"
	"github.com/reeveci/reeve-lib/schema"
)

var CLIMethods = map[string]string{
	"action":  "<action> [<search ...>] - execute action",
	"encrypt": "<secret value> - encrypt variables for usage in .reeve.yaml secrets",
	"rescan":  "rescan all repositories",
}

func (p *GiteaPlugin) CLIMethod(method string, args []string) (string, error) {
	switch method {
	case "action":
		return p.CLIAction(args)

	case "encrypt":
		return p.CLIEncrypt(args)

	case "rescan":
		err := p.API.NotifyMessages([]schema.Message{{Target: PLUGIN_NAME, Options: map[string]string{
			"type":      "operation",
			"operation": "rescan",
		}}})
		if err != nil {
			return "", fmt.Errorf("queueing operation failed - %s", err)
		}
		return "accepted", nil

	default:
		return "", fmt.Errorf("unknown method %s", method)
	}
}

func (p *GiteaPlugin) CLIAction(args []string) (string, error) {
	var action string
	if len(args) > 0 {
		action = args[0]
	}

	if action == "" {
		return "", fmt.Errorf("no action was specified")
	}

	err := p.API.NotifyMessages([]schema.Message{{Target: PLUGIN_NAME, Options: map[string]string{
		"type":   "action",
		"action": action,
		"search": strings.Join(args[1:], " "),
	}}})

	if err != nil {
		return "", fmt.Errorf("queueing action failed - %s", err)
	}
	return "accepted", nil
}

func (p *GiteaPlugin) CLIEncrypt(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("encrypt expects one argument but got %v", len(args))
	}

	encrypted, err := encryption.EncryptSecret(p.SecretKey, args[0])
	if err != nil {
		return "", fmt.Errorf("encryption failed - %s", err)
	}
	return encrypted, nil
}
