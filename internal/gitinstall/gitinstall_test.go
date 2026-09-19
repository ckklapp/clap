package gitinstall

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in        string
		wantClone string
		wantHost  string
		wantName  string
		wantErr   bool
	}{
		{"https://github.com/owner/repo", "https://github.com/owner/repo.git", "github.com", "repo", false},
		{"github.com/owner/repo", "https://github.com/owner/repo.git", "github.com", "repo", false},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git", "github.com", "repo", false},
		{"https://gitlab.com/owner/repo", "https://gitlab.com/owner/repo.git", "gitlab.com", "repo", false},
		{"https://bitbucket.org/owner/repo", "https://bitbucket.org/owner/repo.git", "bitbucket.org", "repo", false},
		{"https://git.example.com/group/sub/repo", "https://git.example.com/group/sub/repo.git", "git.example.com", "repo", false},
		{"owner/repo", "", "", "", true},
		{"repo", "", "", "", true},
		{"", "", "", "", true},
		{"https://github.com/owner", "", "", "", true},
	}
	for _, c := range cases {
		clone, host, name, err := Normalize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Normalize(%q): expected error, got clone=%q host=%q name=%q", c.in, clone, host, name)
			}
			continue
		}
		if err != nil {
			t.Errorf("Normalize(%q): unexpected error: %v", c.in, err)
			continue
		}
		if clone != c.wantClone || host != c.wantHost || name != c.wantName {
			t.Errorf("Normalize(%q) = (%q, %q, %q), want (%q, %q, %q)", c.in, clone, host, name, c.wantClone, c.wantHost, c.wantName)
		}
	}
}
