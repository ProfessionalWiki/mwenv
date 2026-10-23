package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Type string

const (
	Extension Type = "extension"
	Skin      Type = "skin"
)

type Project struct {
	Root string
	Type Type

	// Name is the directory name: the extension.json or skin.json name without spaces, as in "PageApprovals" for
	// "Page Approvals".
	Name string

	DefaultSkinKey string

	// ExternalGitDir is the main repository's git directory when Root is a linked worktree, which git inside the
	// container needs to see at the same path.
	ExternalGitDir string
}

// Dir is the project's directory relative to the MediaWiki installation.
func (p Project) Dir() string {
	if p.Type == Skin {
		return "skins/" + p.Name
	}
	return "extensions/" + p.Name
}

func Detect(dir string) (Project, error) {
	output, err := gitOutput(dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	paths := strings.Split(output, "\n")
	if err != nil || len(paths) != 2 {
		return Project{}, fmt.Errorf("%s is not inside a git worktree", dir)
	}
	root, commonDir := paths[0], paths[1]

	p, err := readManifest(root)
	if err != nil {
		return Project{}, err
	}

	p.ExternalGitDir = externalGitDir(root, commonDir)
	return p, nil
}

func readManifest(root string) (Project, error) {
	if content, err := os.ReadFile(filepath.Join(root, "extension.json")); err == nil {
		return parseManifest(root, Extension, content)
	}

	if content, err := os.ReadFile(filepath.Join(root, "skin.json")); err == nil {
		return parseManifest(root, Skin, content)
	}

	return Project{}, fmt.Errorf("%s contains neither extension.json nor skin.json", root)
}

func parseManifest(root string, projectType Type, content []byte) (Project, error) {
	var manifest struct {
		Name           string          `json:"name"`
		ValidSkinNames json.RawMessage `json:"ValidSkinNames"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return Project{}, fmt.Errorf("cannot parse %s.json in %s: %w", projectType, root, err)
	}

	name := strings.Join(strings.Fields(manifest.Name), "")
	if !isDirectoryName(name) {
		return Project{}, fmt.Errorf("%s.json in %s has no usable name: %q", projectType, root, manifest.Name)
	}

	p := Project{Root: root, Type: projectType, Name: name}

	if projectType == Skin {
		p.DefaultSkinKey = defaultSkinKey(manifest.ValidSkinNames, name)
	}

	return p, nil
}

func isDirectoryName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// defaultSkinKey is the first key of ValidSkinNames, falling back to the lowercased name, as MediaWiki's installer
// does, for skins that register their key in code.
func defaultSkinKey(validSkinNames json.RawMessage, name string) string {
	if key, found := firstKey(validSkinNames); found {
		return key
	}
	return strings.ToLower(name)
}

func firstKey(object json.RawMessage) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(object))

	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return "", false
	}

	token, err := decoder.Token()
	if err != nil {
		return "", false
	}

	key, isKey := token.(string)
	return key, isKey
}

func externalGitDir(root string, commonDir string) string {
	relative, err := filepath.Rel(root, commonDir)
	if err == nil && filepath.IsLocal(relative) {
		return ""
	}
	return commonDir
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}
