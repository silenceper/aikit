package skill

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const skillFileName = "SKILL.md"

// Info holds a discovered skill's name, description, and path.
type Info struct {
	Name string
	Desc string
	Dir  string
}

// Discover finds all SKILL.md files under root and returns skill infos.
func Discover(root string) ([]Info, error) {
	var out []Info
	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		if fi.Name() != skillFileName {
			return nil
		}
		dir := filepath.Dir(path)
		name := nameFromDir(dir)
		desc := ""
		if content, err := os.ReadFile(path); err == nil {
			if n := nameFromFrontmatter(content); n != "" {
				name = n
			}
			desc = descFromContent(content)
		}
		out = append(out, Info{Name: name, Desc: desc, Dir: dir})
		return nil
	})
	return out, err
}

func nameFromDir(dir string) string {
	return filepath.Base(dir)
}

var frontmatterNameRe = regexp.MustCompile(`(?m)^name:\s*["']?([^"'\n]+)["']?\s*$`)
var frontmatterDescRe = regexp.MustCompile(`(?m)^description:\s*["']?([^"'\n]+)["']?\s*$`)

func nameFromFrontmatter(content []byte) string {
	m := frontmatterNameRe.FindSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

// descFromContent extracts description from frontmatter or first markdown heading/paragraph.
func descFromContent(content []byte) string {
	m := frontmatterDescRe.FindSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(string(m[1]))
	}
	// Fallback: first non-empty, non-frontmatter line after frontmatter
	inFrontmatter := false
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			inFrontmatter = !inFrontmatter
			continue
		}
		if inFrontmatter {
			continue
		}
		if trimmed == "" {
			continue
		}
		// Strip markdown heading prefix
		trimmed = strings.TrimLeft(trimmed, "# ")
		if trimmed != "" {
			if len(trimmed) > 80 {
				trimmed = trimmed[:80] + "..."
			}
			return trimmed
		}
	}
	return ""
}
