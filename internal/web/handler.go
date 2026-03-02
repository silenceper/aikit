package web

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/catalog"
	"github.com/silenceper/aikit/internal/discovery"
	"github.com/silenceper/aikit/internal/skill"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/pkg/config"
)

func handleGetCatalog(c *gin.Context) {
	cfg, err := config.LoadCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func handleAddEntry(c *gin.Context) {
	kind := c.Param("kind")

	var entry asset.CatalogEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if entry.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	addFn, ok := addFuncForKind(kind)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid kind: " + kind})
		return
	}

	if err := addFn(entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "added"})
}

func handleUpdateEntry(c *gin.Context) {
	kind := c.Param("kind")
	name := c.Param("name")

	var entry asset.CatalogEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entry.Name = name

	addFn, ok := addFuncForKind(kind)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid kind: " + kind})
		return
	}

	if err := addFn(entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func handleDeleteEntry(c *gin.Context) {
	kind := c.Param("kind")
	name := c.Param("name")

	removeFn, ok := removeFuncForKind(kind)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid kind: " + kind})
		return
	}

	found, err := removeFn(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

type addFunc func(asset.CatalogEntry) error
type removeFunc func(string) (bool, error)

func addFuncForKind(kind string) (addFunc, bool) {
	switch kind {
	case "skills":
		return catalog.AddSkill, true
	case "rules":
		return catalog.AddRule, true
	case "mcps":
		return catalog.AddMCP, true
	case "commands":
		return catalog.AddCommand, true
	default:
		return nil, false
	}
}

func removeFuncForKind(kind string) (removeFunc, bool) {
	switch kind {
	case "skills":
		return catalog.RemoveSkill, true
	case "rules":
		return catalog.RemoveRule, true
	case "mcps":
		return catalog.RemoveMCP, true
	case "commands":
		return catalog.RemoveCommand, true
	default:
		return nil, false
	}
}

type discoverRequest struct {
	Source string `json:"source"`
}

type discoveredItem struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

func handleDiscover(c *gin.Context) {
	var req discoverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	src := strings.TrimSpace(req.Source)
	if src == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source is required"})
		return
	}

	repoDir, resolvedSrc, err := resolveSource(src)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var items []discoveredItem

	skills, _ := skill.Discover(repoDir)
	for _, s := range skills {
		items = append(items, discoveredItem{Kind: "skill", Name: s.Name, Desc: s.Desc})
	}

	assets, _ := discovery.DiscoverAll(repoDir)
	for _, a := range assets {
		items = append(items, discoveredItem{Kind: a.Kind, Name: a.Name, Desc: a.Desc})
	}

	c.JSON(http.StatusOK, gin.H{
		"source": resolvedSrc,
		"items":  items,
	})
}

type batchAddRequest struct {
	Source string `json:"source"`
	Group  string `json:"group"`
	Items  []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		Desc string `json:"desc"`
	} `json:"items"`
}

func handleBatchAdd(c *gin.Context) {
	var req batchAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	group := strings.TrimSpace(req.Group)
	if group == "" {
		group = "Ungrouped"
	}

	added := 0
	for _, item := range req.Items {
		entry := asset.CatalogEntry{
			Name:        item.Name,
			Source:      req.Source,
			Description: item.Desc,
			Group:       group,
		}
		var addFn addFunc
		switch item.Kind {
		case "skill":
			addFn = catalog.AddSkill
		case "rule":
			addFn = catalog.AddRule
		case "mcp":
			addFn = catalog.AddMCP
		case "command":
			addFn = catalog.AddCommand
		default:
			continue
		}
		if err := addFn(entry); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to add %s %s: %v", item.Kind, item.Name, err)})
			return
		}
		added++
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("added %d asset(s)", added)})
}

func resolveSource(src string) (repoDir, resolvedSrc string, err error) {
	if isLocalPath(src) {
		abs, e := filepath.Abs(src)
		if e != nil {
			return "", "", e
		}
		return abs, config.LocalSourceID(), nil
	}
	cacheDir, e := config.CacheDir()
	if e != nil {
		return "", "", e
	}
	subdir := source.NormalizeSource(src)
	if subdir == "" {
		return "", "", fmt.Errorf("could not normalize source %q", src)
	}
	dest := filepath.Join(cacheDir, subdir)
	if err := source.CloneOrFetch(src, dest); err != nil {
		return "", "", err
	}
	return dest, src, nil
}

func isLocalPath(s string) bool {
	return s == "." || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "/")
}
