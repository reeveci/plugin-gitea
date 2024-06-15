package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// Fetch file content from a repository's file.
// If the file was not found, response and error are nil.
// If a response is returned, its body must be closed by the user.
func (p *GiteaPlugin) FetchRepoFileContent(repository string, file string, ref string) (*http.Response, error) {
	reponame, err := pathEscapeRepository(repository)
	if err != nil {
		return nil, err
	}

	if file == "" {
		return nil, fmt.Errorf("no file specified")
	}

	urlname := fmt.Sprintf("%sapi/v1/repos/%s/raw/%s", p.InternalUrl, reponame, url.PathEscape(file))
	if ref != "" {
		urlname += "?ref=" + url.QueryEscape(ref)
	}

	req, err := http.NewRequest(http.MethodGet, urlname, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.Token))

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

var ReeveFileExtensions = []string{".yaml", ".yml", ".yaml.tmpl", ".yml.tmpl"}

func IsTemplate(file string) bool {
	return strings.HasSuffix(file, ".tmpl")
}

func FindReeveFile(entries []FileResponse) string {
	reeveFiles := make([]*FileResponse, len(ReeveFileExtensions))

	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}

		for i, extension := range ReeveFileExtensions {
			if entry.Name == ".reeve"+extension {
				reeveFiles[i] = &entry
				break
			}
		}
	}

	for _, f := range reeveFiles {
		if f != nil {
			return f.Name
		}
	}

	return ""
}

func FindReadmeFile(entries []FileResponse) string {
	exts := []string{".md", ".txt", ""}

	readmeFiles := make([]*FileResponse, len(exts)+1)

	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}

		if i, ok := isReadmeFileExtension(entry.Name, exts...); ok {
			if readmeFiles[i] == nil || naturalSortLess(readmeFiles[i].Name, entry.Name) {
				readmeFiles[i] = &entry
			}
		}
	}

	for _, f := range readmeFiles {
		if f != nil {
			return f.Name
		}
	}

	return ""
}

// isReadmeFileExtension reports whether name looks like a README file
// based on its name. It will look through the provided extensions and check if the file matches
// one of the extensions and provide the index in the extension list.
// If the filename is `readme.` with an unmatched extension it will match with the index equaling
// the length of the provided extension list.
// Note that the '.' should be provided in ext, e.g ".md"
func isReadmeFileExtension(name string, ext ...string) (int, bool) {
	name = strings.ToLower(name)
	if len(name) < 6 || name[:6] != "readme" {
		return 0, false
	}

	for i, extension := range ext {
		extension = strings.ToLower(extension)
		if name[6:] == extension {
			return i, true
		}
	}

	if name[6] == '.' {
		return len(ext), true
	}

	return 0, false
}

// naturalSortLess compares two strings so that they could be sorted in natural order
func naturalSortLess(s1, s2 string) bool {
	c := collate.New(language.English, collate.Numeric)
	return c.CompareString(s1, s2) < 0
}
