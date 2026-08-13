package library

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Candidate struct {
	Name         string
	Description  string
	Root         string
	RelativePath string
	Hash         string
}

// Discover finds every directory containing SKILL.md below root. If root is a
// skill itself, it is returned as the sole candidate.
func Discover(root string) ([]Candidate, error) {
	return discover(root, true)
}

// DiscoverGit includes a repository-root skill and any nested skills. Local
// path discovery intentionally treats a root SKILL.md as one skill.
func DiscoverGit(root string) ([]Candidate, error) {
	return discover(root, false)
}

func discover(root string, rootOnly bool) ([]Candidate, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve discovery root: %w", err)
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("discovery root is not a directory")
	}

	if hasSkillFile(filepath.Join(resolvedRoot, "SKILL.md")) && rootOnly {
		candidate, err := candidateFromRoot(resolvedRoot, resolvedRoot)
		if err != nil {
			return nil, err
		}
		return []Candidate{candidate}, nil
	}

	var candidates []Candidate
	err = filepath.WalkDir(resolvedRoot, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		candidate, err := candidateFromRoot(resolvedRoot, filepath.Dir(current))
		if err != nil {
			return err
		}
		candidates = append(candidates, candidate)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].RelativePath < candidates[j].RelativePath })
	return candidates, nil
}

func candidateFromRoot(discoveryRoot, skillRoot string) (Candidate, error) {
	if !isWithin(discoveryRoot, skillRoot) {
		return Candidate{}, fmt.Errorf("skill root escapes discovery root")
	}
	skillFile := filepath.Join(skillRoot, "SKILL.md")
	if err := validateContainedFile(discoveryRoot, skillFile); err != nil {
		return Candidate{}, err
	}
	metadata, err := readSkillMetadata(discoveryRoot, skillFile)
	if err != nil {
		return Candidate{}, err
	}
	name := metadata.Name
	if !metadata.NamePresent {
		name = filepath.Base(skillRoot)
	}
	if err := validateSkillName(name); err != nil {
		return Candidate{}, err
	}
	rel, err := filepath.Rel(discoveryRoot, skillRoot)
	if err != nil {
		return Candidate{}, err
	}
	hash, err := HashSkill(skillRoot)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Name: name, Description: metadata.Description, Root: skillRoot, RelativePath: filepath.ToSlash(rel), Hash: hash}, nil
}

type skillMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	NamePresent bool   `yaml:"-"`
}

func readSkillMetadata(root, path string) (skillMetadata, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return skillMetadata{}, fmt.Errorf("resolve SKILL.md: %w", err)
	}
	if !isWithin(root, resolved) {
		return skillMetadata{}, fmt.Errorf("SKILL.md %q escapes discovery root", path)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return skillMetadata{}, err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return skillMetadata{}, fmt.Errorf("inspect SKILL.md: %w", err)
	}
	content, err := secureReadRegular(root, relative, info)
	if err != nil {
		return skillMetadata{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return skillMetadata{}, nil
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	closing := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return skillMetadata{}, fmt.Errorf("SKILL.md has unterminated frontmatter")
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:closing], "\n")), &document); err != nil {
		return skillMetadata{}, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	var metadata skillMetadata
	if err := document.Decode(&metadata); err != nil {
		return skillMetadata{}, fmt.Errorf("decode SKILL.md frontmatter: %w", err)
	}
	if len(document.Content) > 0 {
		mapping := document.Content[0]
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == "name" {
				metadata.NamePresent = true
				if mapping.Content[i+1].Tag == "!!null" || metadata.Name == "" {
					return skillMetadata{}, fmt.Errorf("frontmatter name must not be empty")
				}
			}
		}
	}
	return metadata, nil
}

func validateSkillName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("unsafe skill name %q", name)
	}
	return nil
}

func hasSkillFile(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func validateContainedFile(root, file string) error {
	info, err := os.Lstat(file)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("SKILL.md is not a regular file")
	}
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil {
		return err
	}
	if !isWithin(root, resolved) {
		return fmt.Errorf("SKILL.md %q escapes discovery root", file)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil || !resolvedInfo.Mode().IsRegular() {
		return fmt.Errorf("SKILL.md is not a regular file")
	}
	return nil
}
