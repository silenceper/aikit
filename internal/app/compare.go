package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"sort"

	"github.com/silenceper/aikit/pkg/config"
)

type compareFile struct {
	file      SkillFile
	digest    [sha256.Size]byte
	content   string
	truncated bool
}

func (a *App) Compare(ctx context.Context, request CompareRequest) (CompareResult, error) {
	if request.LeftSkillID == "" || request.RightSkillID == "" {
		return CompareResult{}, fmt.Errorf("two library skills are required")
	}
	if err := ctx.Err(); err != nil {
		return CompareResult{}, err
	}
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return CompareResult{}, err
	}
	leftSkill, err := findSkill(cfg, request.LeftSkillID)
	if err != nil {
		return CompareResult{}, err
	}
	rightSkill, err := findSkill(cfg, request.RightSkillID)
	if err != nil {
		return CompareResult{}, err
	}
	leftFiles, err := a.compareFiles(ctx, leftSkill.ID)
	if err != nil {
		return CompareResult{}, fmt.Errorf("read left skill: %w", err)
	}
	rightFiles, err := a.compareFiles(ctx, rightSkill.ID)
	if err != nil {
		return CompareResult{}, fmt.Errorf("read right skill: %w", err)
	}
	result := CompareResult{
		Left:  CompareSide{Skill: leftSkill, Files: compareFileList(leftFiles)},
		Right: CompareSide{Skill: rightSkill, Files: compareFileList(rightFiles)},
	}
	result.Metadata = compareMetadata(leftSkill, rightSkill)
	paths := make(map[string]struct{}, len(leftFiles)+len(rightFiles))
	for path := range leftFiles {
		paths[path] = struct{}{}
	}
	for path := range rightFiles {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		left, leftOK := leftFiles[path]
		right, rightOK := rightFiles[path]
		switch {
		case !rightOK:
			result.Files = append(result.Files, FileDiff{Path: path, Kind: FileOnlyLeft})
			if left.file.Kind == SkillFileRegular {
				result.Content = append(result.Content, ContentDiff{Path: path, Left: left.content, LeftTruncated: left.truncated})
			}
		case !leftOK:
			result.Files = append(result.Files, FileDiff{Path: path, Kind: FileOnlyRight})
			if right.file.Kind == SkillFileRegular {
				result.Content = append(result.Content, ContentDiff{Path: path, Right: right.content, RightTruncated: right.truncated})
			}
		case left.file.Kind != right.file.Kind || left.file.Size != right.file.Size || left.digest != right.digest:
			result.Files = append(result.Files, FileDiff{Path: path, Kind: FileChanged})
			if left.file.Kind == SkillFileRegular || right.file.Kind == SkillFileRegular {
				result.Content = append(result.Content, ContentDiff{
					Path: path, Left: left.content, Right: right.content,
					LeftTruncated: left.truncated, RightTruncated: right.truncated,
				})
			}
		}
	}
	result.Equal = len(result.Metadata) == 0 && len(result.Files) == 0
	return result, nil
}

func (a *App) compareFiles(ctx context.Context, id string) (map[string]compareFile, error) {
	root, err := a.deps.OpenSkillRoot(a.deps.Paths.LibrarySkills, id)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	result := map[string]compareFile{}
	err = fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		item := compareFile{file: SkillFile{Path: path}}
		switch {
		case entry.IsDir():
			item.file.Kind = SkillFileDirectory
		case entry.Type().IsRegular():
			item.file.Kind = SkillFileRegular
			file, err := root.Open(path)
			if err != nil {
				return err
			}
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				_ = file.Close()
				return fmt.Errorf("file %q changed type while comparing", path)
			}
			item.file.Size = info.Size()
			item.digest, item.content, item.truncated, err = readComparedFile(file)
			closeErr := file.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		case entry.Type()&fs.ModeSymlink != 0:
			item.file.Kind = SkillFileSymlink
			target, err := root.Readlink(path)
			if err != nil {
				return err
			}
			item.file.LinkTarget = target
			item.file.Size = int64(len(target))
			// Domain-separate link payloads from regular file contents.
			item.digest = sha256.Sum256(append([]byte("aikit:symlink\x00"), []byte(target)...))
			item.content = target
		default:
			item.file.Kind = SkillFileOther
		}
		result[path] = item
		return nil
	})
	return result, err
}

func readComparedFile(reader io.Reader) ([sha256.Size]byte, string, bool, error) {
	h := sha256.New()
	prefix, err := io.ReadAll(io.LimitReader(reader, SkillPreviewByteLimit+1))
	if err != nil {
		return [sha256.Size]byte{}, "", false, err
	}
	if _, err := h.Write(prefix); err != nil {
		return [sha256.Size]byte{}, "", false, err
	}
	if _, err := io.Copy(h, reader); err != nil {
		return [sha256.Size]byte{}, "", false, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	truncated := len(prefix) > SkillPreviewByteLimit
	if truncated {
		prefix = prefix[:SkillPreviewByteLimit]
	}
	return digest, string(prefix), truncated, nil
}

func compareFileList(values map[string]compareFile) []SkillFile {
	result := make([]SkillFile, 0, len(values))
	for _, item := range values {
		result = append(result, item.file)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func compareMetadata(left, right config.Skill) []MetadataDiff {
	fields := []struct {
		name        string
		left, right string
	}{
		{"id", left.ID, right.ID},
		{"name", left.Name, right.Name},
		{"description", left.Description, right.Description},
		{"source", left.Source, right.Source},
		{"source_path", left.SourcePath, right.SourcePath},
		{"ref", formatRef(left.Ref), formatRef(right.Ref)},
		{"resolved", left.Resolved, right.Resolved},
		{"hash", left.Hash, right.Hash},
	}
	result := make([]MetadataDiff, 0, len(fields))
	for _, field := range fields {
		if field.left != field.right {
			result = append(result, MetadataDiff{Field: field.name, Left: field.left, Right: field.right})
		}
	}
	return result
}

func formatRef(ref *config.Ref) string {
	if ref == nil {
		return ""
	}
	return ref.Kind + ":" + ref.Value
}
