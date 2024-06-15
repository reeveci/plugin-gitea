package main

import (
	"fmt"
	"net/url"
	"strings"
)

func boolSetting(settings map[string]string, key string) (result bool, err error) {
	value := settings[key]
	switch strings.ToLower(value) {
	case "true":
		return true, nil

	case "false", "":
		return false, nil

	default:
		return false, fmt.Errorf("invalid boolean setting %s: %s", key, value)
	}
}

func requireSetting(settings map[string]string, key string) (result string, err error) {
	result = settings[key]
	if result == "" {
		err = fmt.Errorf("missing required setting %s", key)
	}
	return
}

func defaultSetting(settings map[string]string, key, defaultValue string) (result string) {
	result = settings[key]
	if result == "" {
		result = defaultValue
	}
	return
}

func parseBaseURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL \"%s\" - %s", baseURL, err)
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.JoinPath("/").String(), nil
}

func collectFiles(webhook Webhook) []string {
	fileMap := make(map[string]struct{ Before, Now bool })
	for i := len(webhook.Commits) - 1; i >= 0; i-- {
		commit := webhook.Commits[i]

		for _, file := range commit.Added {
			item, ok := fileMap[file]
			if !ok {
				item.Before = false
			}
			item.Now = true
			fileMap[file] = item
		}

		for _, file := range commit.Modified {
			item, ok := fileMap[file]
			if !ok {
				item.Before = true
			}
			item.Now = true
			fileMap[file] = item
		}

		for _, file := range commit.Removed {
			item, ok := fileMap[file]
			if !ok {
				item.Before = true
			}
			item.Now = false
			fileMap[file] = item
		}
	}

	files := make([]string, 0, len(fileMap))
	for file, state := range fileMap {
		if state.Before || state.Now {
			files = append(files, file)
		}
	}
	return files
}

func pathEscapeRepository(repository string) (string, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed repository identifier \"%s\"", repository)
	}
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("malformed repository identifier \"%s\"", repository)
		}
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/"), nil
}
