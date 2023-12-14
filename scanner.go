package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/mileusna/crontab"
	"gopkg.in/yaml.v3"
)

func NewScanner(plugin *GiteaPlugin) (*Scanner, error) {
	s := &Scanner{
		plugin: plugin,
		queue:  make(chan ScanRequest, 10),
		cron:   crontab.New(),
	}

	if s.plugin.DiscoverySchedule == "never" {
		s.plugin.Log.Info("scheduled discovery scans are disabled")
	} else {
		err := s.cron.AddJob(s.plugin.DiscoverySchedule, func() {
			s.plugin.Log.Info("triggering scheduled discovery scan")
			s.Scan()
		})
		if err != nil {
			return nil, fmt.Errorf("error setting up scheduled discovery scans - %s", err)
		}
		s.plugin.Log.Info(fmt.Sprintf("scheduled discovery scans are configured at \"%s\"", s.plugin.DiscoverySchedule))
	}

	go s.handleQueue()

	return s, nil
}

type Scanner struct {
	plugin *GiteaPlugin
	queue  chan ScanRequest
	cron   *crontab.Crontab

	lock       sync.Mutex
	closed     bool
	knownRepos map[string]bool
}

func (s *Scanner) Close() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.closed = true

	s.cron.Clear()
	s.cron.Shutdown()

	close(s.queue)
}

func (s *Scanner) Search(search string) (searchResponse SearchResponse, err error) {
	searchUrl := fmt.Sprintf("%sapi/v1/repos/search", s.plugin.InternalUrl)
	query := url.Values{}

	if !s.plugin.Unrestricted {
		userUrl := fmt.Sprintf("%sapi/v1/user", s.plugin.InternalUrl)
		req, err := http.NewRequest(http.MethodGet, userUrl, nil)
		if err != nil {
			return searchResponse, fmt.Errorf("determining user failed - %s", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

		resp, err := s.plugin.http.Do(req)
		if err != nil {
			return searchResponse, fmt.Errorf("determining user failed - %s", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return searchResponse, fmt.Errorf("determining user failed - %s", string(body))
		}

		var userResponse UserResponse
		err = json.NewDecoder(resp.Body).Decode(&userResponse)
		if err != nil {
			return searchResponse, fmt.Errorf("error parsing user response - %s", err)
		}

		query.Set("uid", strconv.Itoa(userResponse.ID))
	}

	if search != "" {
		query.Set("q", search)
	}

	if len(query) > 0 {
		searchUrl += fmt.Sprintf("?%s", query.Encode())
	}

	req, err := http.NewRequest(http.MethodGet, searchUrl, nil)
	if err != nil {
		return searchResponse, fmt.Errorf("searching repositories failed - %s", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err := s.plugin.http.Do(req)
	if err != nil {
		return searchResponse, fmt.Errorf("searching repositories failed - %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return searchResponse, nil
	}

	err = json.NewDecoder(resp.Body).Decode(&searchResponse)
	if err != nil {
		return searchResponse, fmt.Errorf("error parsing search response - %s", err)
	}

	return
}

func (s *Scanner) FetchCommit(repo, branch string) (*CommitResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%sapi/v1/repos/%s/branches/%s", s.plugin.InternalUrl, repo, branch), nil)
	if err != nil {
		return nil, fmt.Errorf("fetching commit failed - %s", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err := s.plugin.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching commit failed - %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var commitResponse CommitResponse
	err = json.NewDecoder(resp.Body).Decode(&commitResponse)
	if err != nil {
		return nil, fmt.Errorf("error parsing commit response - %s", err)
	}

	return &commitResponse, nil
}

func (s *Scanner) TestRepositoryAccess(repository string) (bool, error) {
	userUrl := fmt.Sprintf("%sapi/v1/user", s.plugin.InternalUrl)
	req, err := http.NewRequest(http.MethodGet, userUrl, nil)
	if err != nil {
		return false, fmt.Errorf("determining user failed - %s", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err := s.plugin.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("determining user failed - %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("determining user failed - %s", string(body))
	}

	var userResponse UserResponse
	err = json.NewDecoder(resp.Body).Decode(&userResponse)
	if err != nil {
		return false, fmt.Errorf("error parsing user response - %s", err)
	}

	assigneesUrl := fmt.Sprintf("%sapi/v1/repos/%s/assignees", s.plugin.InternalUrl, repository)
	req, err = http.NewRequest(http.MethodGet, assigneesUrl, nil)
	if err != nil {
		return false, fmt.Errorf("determining assignees failed - %s", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err = s.plugin.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("determining assignees failed - %s", err)
	}
	defer resp.Body.Close()

	// if we are not allowed to access the repository, the server responds with status 404
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("determining assignees failed - %s", string(body))
	}

	var assigneesResponse AssigneesResponse
	err = json.NewDecoder(resp.Body).Decode(&assigneesResponse)
	if err != nil {
		return false, fmt.Errorf("error parsing assignees response - %s", err)
	}

	var found bool
	for _, assignee := range assigneesResponse {
		if userResponse.ID == assignee.ID {
			found = true
			break
		}
	}

	return found, nil
}

func (s *Scanner) ScanRepository(repository, commit string, scanners ...DocumentScanner) error {
	if len(scanners) == 0 {
		return nil
	}

	defer func() {
		for _, scanner := range scanners {
			if scanner != nil {
				scanner.Close()
			}
		}
	}()

	if !s.plugin.Unrestricted {
		ok, err := s.TestRepositoryAccess(repository)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}

	configUrl := fmt.Sprintf("%sapi/v1/repos/%s/raw/.reeve.yaml", s.plugin.InternalUrl, repository)
	if commit != "" {
		configUrl = fmt.Sprintf("%s?ref=%s", configUrl, commit)
	}

	req, err := http.NewRequest(http.MethodGet, configUrl, nil)
	if err != nil {
		return fmt.Errorf("fetching .reeve.yaml from repository failed - %s", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err := s.plugin.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetching .reeve.yaml from repository failed - %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	decoder := yaml.NewDecoder(resp.Body)
	for {
		var document Document
		err = decoder.Decode(&document)

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("error parsing .reeve.yaml from repository %s - %s", repository, err)
		}

		for _, scanner := range scanners {
			if scanner != nil {
				err := scanner.Scan(document)
				if err != nil {
					return err
				}
			}
		}
	}

	for _, scanner := range scanners {
		if scanner != nil {
			scanner.Done()
		}
	}

	return nil
}

type ScanRequest struct {
	Type, Repository string
}

func (s *Scanner) Scan() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.closed {
		return
	}

	s.queue <- ScanRequest{Type: "scan"}
}

func (s *Scanner) Notify(repository string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.closed {
		return
	}

	s.queue <- ScanRequest{Type: "notify", Repository: repository}
}

func (s *Scanner) handleQueue() {
	for {
		request, ok := <-s.queue
		if !ok {
			return
		}

		switch request.Type {
		case "scan":
			s.scan()

		case "notify":
			s.notify(request.Repository)
		}
	}
}

func (s *Scanner) scan() {
	s.plugin.Log.Info("starting discovery scan")

	searchResponse, err := s.Search("")
	if err != nil {
		s.plugin.Log.Error(err.Error())
		return
	}

	currentRepos := make(map[string]bool, len(searchResponse.Data))
	for _, repo := range searchResponse.Data {
		currentRepos[repo.FullName] = true
	}
	for repository := range s.knownRepos {
		if !currentRepos[repository] {
			s.plugin.Log.Info(fmt.Sprintf("dropping repository %s", repository))
			SendActionBundleMessage(s.plugin, ActionBundle{
				BundleID: "repo:" + repository,
				Actions:  nil,
			})
		}
	}
	s.knownRepos = currentRepos

	for _, repo := range searchResponse.Data {
		s.notify(repo.FullName)
	}
}

func (s *Scanner) notify(repository string) {
	if repository == "" {
		return
	}

	s.plugin.Log.Info(fmt.Sprintf("scanning repository %s", repository))

	scanners := make([]DocumentScanner, 0, 2)

	s.plugin.Lock()
	hasUI := s.plugin.WebUIPresent
	s.plugin.Unlock()
	if hasUI {
		scanners = append(scanners, NewWebUIScanner(s.plugin, repository))
	}

	scanners = append(scanners, NewCronScanner(s.plugin, repository))

	s.ScanRepository(repository, "", scanners...)
}

type DocumentScanner interface {
	Scan(document Document) error
	Done()
	Close()
}
