package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/reeveci/plugin-gitea/encryption"
	"github.com/reeveci/reeve-lib/conditions"
	"github.com/reeveci/reeve-lib/schema"
)

func NewDiscoverScanner(plugin *GiteaPlugin, repository string, commit string, env map[string]schema.Env, pipelines *[]*schema.PipelineDefinition, defaultConditions map[string]schema.Condition) DocumentScanner {
	return &DiscoverScanner{
		plugin:            plugin,
		repository:        repository,
		commit:            commit,
		env:               env,
		pipelines:         pipelines,
		defaultConditions: defaultConditions,
	}
}

type DiscoverScanner struct {
	plugin            *GiteaPlugin
	repository        string
	commit            string
	env               map[string]schema.Env
	pipelines         *[]*schema.PipelineDefinition
	defaultConditions map[string]schema.Condition

	readme string
}

func (s *DiscoverScanner) Init(rootFiles []FileResponse) error {
	if readmeFile := FindReadmeFile(rootFiles); readmeFile != "" {
		readmeResp, err := s.plugin.FetchRepoFileContent(s.repository, readmeFile, s.commit)
		if err != nil {
			return fmt.Errorf("fetching %s from repository %s failed - %s", readmeFile, s.repository, err)
		}

		if readmeResp != nil {
			content, err := io.ReadAll(readmeResp.Body)
			readmeResp.Body.Close()
			if err != nil {
				return fmt.Errorf("fetching %s from repository %s failed - %s", readmeFile, s.repository, err)
			}
			s.readme = strings.ReplaceAll(strings.ReplaceAll(string(content), "\r\n", "\n"), "\r", "\n")

			if strings.TrimSpace(s.readme) != "" && !strings.HasSuffix(strings.ToLower(readmeFile), ".md") {
				s.readme = "    " + strings.ReplaceAll(s.readme, "\n", "\n    ")
			}
		}
	}

	return nil
}

func (s *DiscoverScanner) Scan(document *SourceDocument) error {
	switch document.Type {
	case "pipeline":
		pipeline := document.PipelineDefinition

		if s.readme != "" {
			if pipeline.Description != "" {
				pipeline.Description += "\n\n---\n\n"
			}
			pipeline.Description += s.readme
		}

		conditions.ApplyDefaults(&pipeline.When, s.defaultConditions)

		*s.pipelines = append(*s.pipelines, &pipeline)

	case "variable":
		s.env[document.Name] = schema.Env{
			Value:    document.Value,
			Priority: 0,
			Secret:   false,
		}

	case "secret":
		decryptedValue, err := encryption.DecryptSecret(s.plugin.SecretKey, document.Value)
		if err != nil {
			return fmt.Errorf("error decrypting secret %s in %s from repository %s - %s", document.Name, document.SourceFile, s.repository, err)
		}
		s.env[document.Name] = schema.Env{
			Value:    decryptedValue,
			Priority: 0,
			Secret:   true,
		}

	case "trigger":

	default:
		return fmt.Errorf("error parsing %s from repository %s - invalid document type %s", document.SourceFile, s.repository, document.Type)
	}

	return nil
}

func (s *DiscoverScanner) Done() {}

func (s *DiscoverScanner) Close() {}
