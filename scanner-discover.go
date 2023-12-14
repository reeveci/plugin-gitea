package main

import (
	"fmt"

	"github.com/reeveci/plugin-gitea/encryption"
	"github.com/reeveci/reeve-lib/conditions"
	"github.com/reeveci/reeve-lib/schema"
)

func NewDiscoverScanner(plugin *GiteaPlugin, repository string, env map[string]schema.Env, pipelines *[]*schema.PipelineDefinition, defaultConditions map[string]schema.Condition) DocumentScanner {
	return &DiscoverScanner{
		plugin:            plugin,
		repository:        repository,
		env:               env,
		pipelines:         pipelines,
		defaultConditions: defaultConditions,
	}
}

type DiscoverScanner struct {
	plugin            *GiteaPlugin
	repository        string
	env               map[string]schema.Env
	pipelines         *[]*schema.PipelineDefinition
	defaultConditions map[string]schema.Condition
}

func (s *DiscoverScanner) Scan(document Document) error {
	switch document.Type {
	case "pipeline":
		pipeline := document.PipelineDefinition

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
			return fmt.Errorf("error decrypting secret %s from repository %s - %s", document.Name, s.repository, err)
		}
		s.env[document.Name] = schema.Env{
			Value:    decryptedValue,
			Priority: 0,
			Secret:   true,
		}

	case "trigger":

	default:
		return fmt.Errorf("error parsing .reeve.yaml from repository %s - invalid document type %s", s.repository, document.Type)
	}

	return nil
}

func (s *DiscoverScanner) Done() {}

func (s *DiscoverScanner) Close() {}
