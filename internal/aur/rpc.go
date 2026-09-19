package aur

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var rpcBase = "https://aur.archlinux.org/rpc/v5"

type Package struct {
	Name         string   `json:"Name"`
	PackageBase  string   `json:"PackageBase"`
	Version      string   `json:"Version"`
	Description  string   `json:"Description"`
	URL          string   `json:"URL"`
	NumVotes     int      `json:"NumVotes"`
	Popularity   float64  `json:"Popularity"`
	Maintainer   string   `json:"Maintainer"`
	OutOfDate    int      `json:"OutOfDate"`
	Depends      []string `json:"Depends"`
	MakeDepends  []string `json:"MakeDepends"`
	OptDepends   []string `json:"OptDepends"`
	CheckDepends []string `json:"CheckDepends"`
	Conflicts    []string `json:"Conflicts"`
	Keywords     []string `json:"Keywords"`
}

type rpcResponse struct {
	Version     int       `json:"version"`
	Type        string    `json:"type"`
	ResultCount int       `json:"resultcount"`
	Results     []Package `json:"results"`
	Error       string    `json:"error"`
}

func doRPC(u string) (*rpcResponse, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "clap-aur")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("AUR RPC %s returned HTTP %d", u, resp.StatusCode)
	}
	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Type == "error" {
		return nil, fmt.Errorf("AUR RPC error: %s", out.Error)
	}
	return &out, nil
}

func Search(query, by string) ([]Package, error) {
	if by == "" {
		by = "name-desc"
	}
	u := fmt.Sprintf("%s/search/%s?by=%s", rpcBase, url.PathEscape(query), url.QueryEscape(by))
	res, err := doRPC(u)
	if err != nil {
		return nil, err
	}
	return res.Results, nil
}

func Info(names []string) ([]Package, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var q strings.Builder
	q.WriteString(rpcBase + "/info?")
	for i, n := range names {
		if i > 0 {
			q.WriteByte('&')
		}
		q.WriteString("arg[]=" + url.QueryEscape(n))
	}
	res, err := doRPC(q.String())
	if err != nil {
		return nil, err
	}
	return res.Results, nil
}

func InfoOne(name string) (*Package, error) {
	pkgs, err := Info([]string{name})
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("package %q not found in the AUR", name)
	}
	return &pkgs[0], nil
}

func StripVersionConstraint(dep string) string {
	for _, sep := range []string{">=", "<=", "==", ">", "<", "="} {
		if i := strings.Index(dep, sep); i >= 0 {
			return dep[:i]
		}
	}
	return dep
}

func ParsePackageName(input string) string {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")

	for _, prefix := range []string{
		"aur.archlinux.org/packages/",
		"aur.archlinux.org/pkgbase/",
	} {
		if i := strings.Index(s, prefix); i >= 0 {
			s = s[i+len(prefix):]
			break
		}
	}

	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}

	return s
}
