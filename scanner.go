package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

func (s *Scanner) Search(search string) (result SearchResult, err error) {
	searchUrl := fmt.Sprintf("%sapi/v1/repos/search", s.plugin.InternalUrl)
	query := url.Values{}

	if !s.plugin.Unrestricted {
		userUrl := fmt.Sprintf("%sapi/v1/user", s.plugin.InternalUrl)
		req, err := http.NewRequest(http.MethodGet, userUrl, nil)
		if err != nil {
			return nil, fmt.Errorf("determining user failed - %s", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

		resp, err := s.plugin.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("determining user failed - %s", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("determining user failed (status %v) - %s", resp.StatusCode, string(body))
		}

		var userResponse UserResponse
		err = json.NewDecoder(resp.Body).Decode(&userResponse)
		if err != nil {
			return nil, fmt.Errorf("error parsing user response - %s", err)
		}

		query.Set("uid", strconv.Itoa(userResponse.ID))
	}

	if search != "" {
		query.Set("q", search)
	}

	page := 1
	for {
		done, err := func() (bool, error) {
			query.Set("page", strconv.Itoa(page))
			pageSearchUrl := fmt.Sprintf("%s?%s", searchUrl, query.Encode())

			req, err := http.NewRequest(http.MethodGet, pageSearchUrl, nil)
			if err != nil {
				return false, fmt.Errorf("searching repositories failed - %s", err)
			}

			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

			resp, err := s.plugin.http.Do(req)
			if err != nil {
				return false, fmt.Errorf("searching repositories failed - %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return true, nil
			}

			var res SearchResponse
			err = json.NewDecoder(resp.Body).Decode(&res)
			if err != nil {
				return false, fmt.Errorf("error parsing search response - %s", err)
			}
			if len(res.Data) == 0 {
				return true, nil
			}

			total, _ := strconv.Atoi(resp.Header.Get("x-total-count"))
			if result == nil {
				result = make(SearchResult, 0, total)
			}
			result = append(result, res.Data...)

			if total > 0 && len(result) >= total {
				return true, nil
			}

			page += 1
			return false, nil
		}()

		if err != nil {
			return nil, err
		}
		if done {
			break
		}
	}

	return
}

func (s *Scanner) FetchCommit(repository, branch string) (*CommitResponse, error) {
	reponame, err := pathEscapeRepository(repository)
	if err != nil {
		return nil, fmt.Errorf("fetching commit from %s failed - %s", repository, err)
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%sapi/v1/repos/%s/branches/%s", s.plugin.InternalUrl, reponame, url.PathEscape(branch)), nil)
	if err != nil {
		return nil, fmt.Errorf("fetching commit from %s failed - %s", repository, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err := s.plugin.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching commit from %s failed - %s", repository, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var commitResponse CommitResponse
	err = json.NewDecoder(resp.Body).Decode(&commitResponse)
	if err != nil {
		return nil, fmt.Errorf("fetching commit from %s failed - %s", repository, err)
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
		return false, fmt.Errorf("determining user failed (status %v) - %s", resp.StatusCode, string(body))
	}

	var userResponse UserResponse
	err = json.NewDecoder(resp.Body).Decode(&userResponse)
	if err != nil {
		return false, fmt.Errorf("determining user failed - %s", err)
	}

	reponame, err := pathEscapeRepository(repository)
	if err != nil {
		return false, fmt.Errorf("determining assignees for %s failed - %s", repository, err)
	}

	assigneesUrl := fmt.Sprintf("%sapi/v1/repos/%s/assignees", s.plugin.InternalUrl, reponame)
	req, err = http.NewRequest(http.MethodGet, assigneesUrl, nil)
	if err != nil {
		return false, fmt.Errorf("determining assignees for %s failed - %s", repository, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err = s.plugin.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("determining assignees for %s failed - %s", repository, err)
	}
	defer resp.Body.Close()

	// if we are not allowed to access the repository, the server responds with status 404
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("determining assignees for %s failed (status %v) - %s", repository, resp.StatusCode, string(body))
	}

	var assigneesResponse AssigneesResponse
	err = json.NewDecoder(resp.Body).Decode(&assigneesResponse)
	if err != nil {
		return false, fmt.Errorf("determining assignees for %s failed - %s", repository, err)
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

func (s *Scanner) FetchRootFiles(repository string, ref string) ([]FileResponse, error) {
	reponame, err := pathEscapeRepository(repository)
	if err != nil {
		return nil, fmt.Errorf("scanning repository %s failed - %s", repository, err)
	}

	urlname := fmt.Sprintf("%sapi/v1/repos/%s/contents", s.plugin.InternalUrl, reponame)
	if ref != "" {
		urlname += "?ref=" + url.QueryEscape(ref)
	}

	req, err := http.NewRequest(http.MethodGet, urlname, nil)
	if err != nil {
		return nil, fmt.Errorf("scanning repository %s failed - %s", repository, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.plugin.Token))

	resp, err := s.plugin.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scanning repository %s failed - %s", repository, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var contentsResponse ContentsResponse
	err = json.NewDecoder(resp.Body).Decode(&contentsResponse)
	if err != nil {
		return nil, fmt.Errorf("scanning repository %s failed - %s", repository, err)
	}

	return contentsResponse, nil
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

	repoRootFiles, err := s.FetchRootFiles(repository, commit)
	if err != nil {
		return err
	}

	configFile := FindReeveFile(repoRootFiles)
	if configFile == "" {
		return nil
	}

	for _, scanner := range scanners {
		if scanner != nil {
			scanner.Init(repoRootFiles)
		}
	}

	documents, err := s.loadRepositoryConfig(repository, configFile, commit, true, nil)
	if err != nil {
		return err
	}

	for _, document := range documents {
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

func (s *Scanner) loadRepositoryConfig(repository string, configFile string, commit string, ignoreFetchError bool, templateData any) ([]*SourceDocument, error) {
	documents, err := s.readRepositoryConfig(repository, configFile, commit, ignoreFetchError, templateData)
	if err != nil {
		return nil, err
	}

	result := make([]*SourceDocument, 0, len(documents))

	for _, document := range documents {
		switch document.Type {
		case "include":
			if document.Path == "" {
				return nil, fmt.Errorf("error resolving include in %s from repository %s - no path specified", configFile, repository)
			}

			results, err := s.loadRepositoryConfig(repository, document.Path, commit, false, document.TemplateData)
			if err != nil {
				return nil, err
			}
			result = append(result, results...)

		default:
			result = append(result, document)
		}
	}

	return result, nil
}

func (s *Scanner) readRepositoryConfig(repository string, configFile string, commit string, ignoreFetchError bool, templateData any) ([]*SourceDocument, error) {
	var ok bool
	for _, ext := range ReeveFileExtensions {
		if strings.HasSuffix(configFile, ext) {
			ok = true
		}
	}
	if !ok {
		return nil, fmt.Errorf("error loading %s from repository %s - invalid file extension, please use one of %s", configFile, repository, strings.Join(ReeveFileExtensions, ", "))
	}

	resp, err := s.plugin.FetchRepoFileContent(repository, configFile, commit)
	if err != nil {
		return nil, fmt.Errorf("fetching %s from repository %s failed - %s", configFile, repository, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if ignoreFetchError {
			return nil, nil
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetching %s from repository %s failed (status %v) - %s", configFile, repository, resp.StatusCode, string(body))
	}

	var content io.Reader = resp.Body
	if IsTemplate(configFile) {
		template, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading %s from repository %s - %s", configFile, repository, err)
		}

		content, err = ParseTemplate(configFile, string(template), templateData)
		if err != nil {
			return nil, fmt.Errorf("error parsing template %s from repository %s - %s", configFile, repository, err)
		}
	}

	var result []*SourceDocument

	decoder := yaml.NewDecoder(content)
	for {
		document := SourceDocument{SourceFile: configFile}
		err = decoder.Decode(&document.Document)

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("error parsing %s from repository %s - %s", configFile, repository, err)
		}

		result = append(result, &document)
	}

	return result, nil
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

	searchResult, err := s.Search("")
	if err != nil {
		s.plugin.Log.Error(err.Error())
		return
	}

	currentRepos := make(map[string]bool, len(searchResult))
	for _, repo := range searchResult {
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

	for _, repo := range searchResult {
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
	Init(rootFiles []FileResponse) error
	Scan(document *SourceDocument) error
	Done()
	Close()
}
