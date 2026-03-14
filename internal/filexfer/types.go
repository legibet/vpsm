package filexfer

import (
	"os"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Name     string
	Path     string
	IsDir    bool
	IsHidden bool
	IsParent bool
	Mode     os.FileMode
	Size     int64
	ModTime  time.Time
}

func EntryFromFileInfo(fullPath string, info os.FileInfo) Entry {
	name := info.Name()
	return Entry{
		Name:     name,
		Path:     fullPath,
		IsDir:    info.IsDir(),
		IsHidden: strings.HasPrefix(name, "."),
		Mode:     info.Mode(),
		Size:     info.Size(),
		ModTime:  info.ModTime(),
	}
}

func ParentEntry(fullPath string) Entry {
	return Entry{
		Name:     "..",
		Path:     fullPath,
		IsDir:    true,
		IsParent: true,
	}
}

func (e Entry) DisplayName() string {
	if e.IsParent {
		return ".."
	}
	if e.IsDir {
		return e.Name + "/"
	}
	return e.Name
}

func SortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]

		if left.IsParent != right.IsParent {
			return left.IsParent
		}
		if left.IsDir != right.IsDir {
			return left.IsDir
		}

		leftName := strings.ToLower(left.Name)
		rightName := strings.ToLower(right.Name)
		if leftName != rightName {
			return leftName < rightName
		}

		return left.Name < right.Name
	})
}

type TransferProgress struct {
	Direction    string
	CurrentPath  string
	BytesDone    int64
	BytesTotal   int64
	ItemsDone    int
	ItemsTotal   int
	CurrentBytes int64
	CurrentTotal int64
	Completed    bool
}
