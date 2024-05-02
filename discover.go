package main

import (
	"fmt"
	"strings"

	"github.com/reeveci/reeve-lib/schema"
)

func (p *GiteaPlugin) Discover(trigger schema.Trigger) ([]schema.Pipeline, error) {
	eventType := trigger["type"]
	triggerType := trigger["trigger"]
	action := trigger["action"]
	ref := trigger["ref"]
	commit := trigger["commit"]
	commitMessage := trigger["commitMessage"]
	repository := trigger["repository"]
	repositoryURL := trigger["repositoryURL"]
	cloneURL := trigger["cloneURL"]
	defaultBranch := trigger["defaultBranch"]
	rawFiles, hasFiles := trigger["files"]
	files := strings.Split(rawFiles, "\n")

	if eventType != "git" ||
		!strings.HasPrefix(strings.ToLower(repositoryURL), strings.ToLower(p.PublicUrl)) ||
		!strings.HasPrefix(strings.ToLower(cloneURL), strings.ToLower(p.PublicUrl)) {
		return nil, nil
	}

	if triggerType == "" ||
		ref == "" ||
		commit == "" ||
		repository == "" ||
		repositoryURL == "" ||
		cloneURL == "" ||
		defaultBranch == "" {
		return nil, fmt.Errorf("invalid git trigger - missing fields - %s", trigger)
	}

	switch triggerType {
	case "push", "action":

	default:
		return nil, fmt.Errorf("invalid git trigger - unknown trigger type %s", triggerType)
	}

	if !p.RepoRegexp.MatchString(repository) {
		return nil, fmt.Errorf("invalid git trigger - malformed repository identifier %s", repository)
	}

	facts := map[string]schema.Fact{
		"trigger":    {triggerType},
		"action":     nil,
		"ref":        {ref},
		"branch":     nil,
		"file":       nil,
		"tag":        nil,
		"repository": {repository},
	}

	if triggerType == "action" {
		if action == "" {
			return nil, fmt.Errorf("invalid git trigger - missing field action")
		}

		facts["action"] = schema.Fact{action}
	}

	if strings.HasPrefix(ref, "refs/heads/") {
		facts["branch"] = schema.Fact{strings.TrimPrefix(ref, "refs/heads/")}
		if triggerType == "push" {
			facts["trigger"] = append(facts["trigger"], "commit")
		}

		if hasFiles {
			facts["file"] = files
		}
	}

	if strings.HasPrefix(ref, "refs/tags/") {
		facts["tag"] = schema.Fact{strings.TrimPrefix(ref, "refs/tags/")}
		if triggerType == "push" {
			facts["trigger"] = append(facts["trigger"], "tag")
		}
	}

	defaultConditions := map[string]schema.Condition{
		"trigger": {
			Include: []string{"commit"},
		},
		"action": {
			Include: []string{""},
		},
		"branch": {
			Include: []string{defaultBranch},
		},
	}

	var triggerHeadline string
	var triggerDescription string
	switch triggerType {
	case "push":
		if strings.HasPrefix(ref, "refs/heads/") {
			triggerHeadline = fmt.Sprintf("[branch %s]", strings.TrimPrefix(ref, "refs/heads/"))
		} else if strings.HasPrefix(ref, "refs/tags/") {
			triggerHeadline = fmt.Sprintf("[tag %s]", strings.TrimPrefix(ref, "refs/tags/"))
		} else {
			triggerHeadline = fmt.Sprintf("[push %s]", ref)
		}
		if strings.TrimSpace(commitMessage) != "" {
			triggerHeadline += " " + commitMessage
		}
		triggerDescription = fmt.Sprintf("%s: %s", triggerType, ref)

	case "action":
		triggerHeadline = fmt.Sprintf("[action %s]", action)
		triggerDescription = fmt.Sprintf("%s: %s", triggerType, action)

	default:
		triggerHeadline = fmt.Sprintf("[%s]", triggerType)
		triggerDescription = triggerType
	}
	shortCommit := commit
	if len(shortCommit) > 10 {
		shortCommit = shortCommit[:10]
	}
	description := fmt.Sprintf(
		`> [%s](%s) | [%s](%s)\
> %s

`, repository, repositoryURL, shortCommit, repositoryURL+"/src/commit/"+commit, triggerDescription)

	env := make(map[string]schema.Env)
	pipelineDefs := make([]*schema.PipelineDefinition, 0)

	err := p.Scanner.ScanRepository(repository, commit, NewDiscoverScanner(p, repository, env, &pipelineDefs, defaultConditions))
	if err != nil {
		return nil, err
	}

	env["__GIT_TOKEN"] = schema.Env{
		Value:    p.Token,
		Priority: 0,
		Secret:   true,
	}

	pipelines := make([]schema.Pipeline, len(pipelineDefs))
	for i, def := range pipelineDefs {
		pipelines[i] = schema.Pipeline{
			PipelineDefinition: *def,

			Env:            env,
			Facts:          facts,
			TaskDomains:    p.TaskDomains,
			TrustedDomains: p.TrustedDomains,
			TrustedTasks:   p.TrustedTasks,

			Setup: schema.Setup{
				RunConfig: schema.RunConfig{
					Task: p.SetupTask,

					Params: map[string]schema.RawParam{
						"GIT_REPOSITORY": schema.LiteralParam(p.CloneUrl + strings.TrimPrefix(cloneURL, p.PublicUrl)),
						"GIT_PASSWORD":   schema.EnvParam{Env: "__GIT_TOKEN"},
						"GIT_COMMIT":     schema.LiteralParam(commit),
					},
				},
			},
		}

		if strings.TrimSpace(pipelines[i].Headline) == "" {
			pipelines[i].Headline = triggerHeadline
		}
		pipelines[i].Description = description + pipelines[i].Description
	}

	return pipelines, nil
}
