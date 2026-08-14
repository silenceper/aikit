package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) SkillDetail(ctx context.Context, idOrName string) (SkillDetail, error) {
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return SkillDetail{}, err
	}
	skill, err := findSkill(cfg, idOrName)
	if err != nil {
		return SkillDetail{}, err
	}
	root, err := a.deps.OpenSkillRoot(a.deps.Paths.LibrarySkills, skill.ID)
	if err != nil {
		return SkillDetail{}, fmt.Errorf("open skill root: %w", err)
	}
	defer root.Close()

	detail := SkillDetail{Skill: skill, EnabledLocations: affectedScopes(cfg, selectorsForSkill(cfg, skill.ID))}
	detail.Files, err = listSkillFiles(root)
	if err != nil {
		return SkillDetail{}, err
	}
	detail.SkillMD, detail.SkillMDTruncated, err = readSkillPreview(root)
	if err != nil {
		return SkillDetail{}, err
	}
	return detail, nil
}

func (a *App) Configuration(ctx context.Context) (ConfigurationDetail, error) {
	if err := ctx.Err(); err != nil {
		return ConfigurationDetail{}, err
	}
	paths := a.deps.Paths
	if paths.Config == "" || paths.LibrarySkills == "" || paths.Cache == "" {
		var err error
		paths, err = config.DefaultPaths()
		if err != nil {
			return ConfigurationDetail{}, err
		}
	}
	return ConfigurationDetail{Config: paths.Config, Library: paths.LibrarySkills, Cache: paths.Cache}, nil
}

func (a *App) ValidateConfiguration(ctx context.Context) (ConfigurationValidation, error) {
	if err := ctx.Err(); err != nil {
		return ConfigurationValidation{}, err
	}
	paths := a.deps.Paths
	if paths.Config == "" {
		var err error
		paths, err = config.DefaultPaths()
		if err != nil {
			return ConfigurationValidation{}, err
		}
	}
	if _, err := (config.Store{Paths: paths}).Load(ctx); err != nil {
		return ConfigurationValidation{Path: paths.Config}, err
	}
	return ConfigurationValidation{Path: paths.Config, Valid: true}, nil
}

func listSkillFiles(root library.VerifiedSkillRoot) ([]SkillFile, error) {
	var files []SkillFile
	err := fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		item := SkillFile{Path: filepath.ToSlash(path)}
		switch {
		case entry.IsDir():
			item.Kind = SkillFileDirectory
		case entry.Type().IsRegular():
			item.Kind = SkillFileRegular
			file, err := root.Open(path)
			if err != nil {
				return err
			}
			info, statErr := file.Stat()
			_ = file.Close()
			if statErr != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("file changed type while listing")
			}
			item.Size = info.Size()
		case entry.Type()&os.ModeSymlink != 0:
			item.Kind = SkillFileSymlink
		default:
			item.Kind = SkillFileOther
		}
		files = append(files, item)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list skill files: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func readSkillPreview(root library.VerifiedSkillRoot) (string, bool, error) {
	file, err := root.Open("SKILL.md")
	if err != nil {
		return "", false, fmt.Errorf("open SKILL.md: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return "", false, fmt.Errorf("SKILL.md changed type while opening")
	}
	content, err := io.ReadAll(io.LimitReader(file, SkillPreviewByteLimit+1))
	if err != nil {
		return "", false, fmt.Errorf("read SKILL.md: %w", err)
	}
	truncated := len(content) > SkillPreviewByteLimit
	if truncated {
		content = content[:SkillPreviewByteLimit]
	}
	return string(content), truncated, nil
}
