package manifest

type Manifest struct {
	Name    string `json:"name"`
	Repo    string `json:"repo"`
	Tag     string `json:"tag"`
	Entry   string `json:"entry"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Source  string `json:"source"`
	Kind    string `json:"kind"`
	ClapVer string `json:"clap_version"`
}

func ExtFor(kind string) string {
	if kind == "library" {
		return ".clapl"
	}
	return ".clap"
}

const Version = "0.1.0"
