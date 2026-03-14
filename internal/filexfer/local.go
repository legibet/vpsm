package filexfer

import (
	"fmt"
	"os"
	"path/filepath"
)

func ReadLocalDir(path string, showHidden bool) ([]Entry, error) {
	items, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read local directory %q: %w", path, err)
	}

	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		name := item.Name()
		if !showHidden && name != ".." && len(name) > 0 && name[0] == '.' {
			continue
		}

		info, err := item.Info()
		if err != nil {
			return nil, fmt.Errorf("stat local entry %q: %w", filepath.Join(path, name), err)
		}
		entries = append(entries, EntryFromFileInfo(filepath.Join(path, name), info))
	}

	SortEntries(entries)
	return entries, nil
}

func StatLocal(path string) (Entry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Entry{}, fmt.Errorf("stat local path %q: %w", path, err)
	}
	return EntryFromFileInfo(path, info), nil
}

func ExistsLocal(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat local path %q: %w", path, err)
}

func RemoveLocal(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove local path %q: %w", path, err)
	}
	return nil
}

func RenameLocal(oldPath string, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename local path %q to %q: %w", oldPath, newPath, err)
	}
	return nil
}

func MkdirLocal(path string) error {
	if err := os.Mkdir(path, 0o755); err != nil {
		return fmt.Errorf("create local directory %q: %w", path, err)
	}
	return nil
}
