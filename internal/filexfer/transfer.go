package filexfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"
)

type transferItem struct {
	localPath  string
	remotePath string
	size       int64
	mode       os.FileMode
	dir        bool
	modTime    time.Time
}

type transferPlan struct {
	items      []transferItem
	bytesTotal int64
	filesTotal int
}

func (f *RemoteFS) UploadPathContext(ctx context.Context, localPath, remotePath string, progress func(TransferProgress)) error {
	plan, err := buildUploadPlan(localPath, remotePath)
	if err != nil {
		return err
	}

	return f.runTransfer(ctx, "upload", plan, progress, func(item transferItem, state *transferState) error {
		if item.dir {
			return f.client.MkdirAll(item.remotePath)
		}

		return f.uploadFileAtomically(ctx, item, state)
	})
}

func (f *RemoteFS) DownloadPathContext(ctx context.Context, remotePath, localPath string, progress func(TransferProgress)) error {
	plan, err := f.buildDownloadPlan(ctx, remotePath, localPath)
	if err != nil {
		return err
	}

	return f.runTransfer(ctx, "download", plan, progress, func(item transferItem, state *transferState) error {
		if item.dir {
			if err := os.MkdirAll(item.localPath, 0o755); err != nil {
				return fmt.Errorf("create local directory %q: %w", item.localPath, err)
			}
			return nil
		}

		return f.downloadFileAtomically(ctx, item, state)
	})
}

func (f *RemoteFS) uploadFileAtomically(ctx context.Context, item transferItem, state *transferState) (err error) {
	if err := f.client.MkdirAll(path.Dir(item.remotePath)); err != nil {
		return fmt.Errorf("create remote parent directory for %q: %w", item.remotePath, err)
	}

	source, err := os.Open(item.localPath)
	if err != nil {
		return fmt.Errorf("open local file %q: %w", item.localPath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	tempPath := tempRemotePath(item.remotePath)
	_ = f.client.Remove(tempPath)

	target, err := f.clientWriter(tempPath, item.mode)
	if err != nil {
		return err
	}

	cleanupTemp := func(baseErr error) error {
		removeErr := f.client.Remove(tempPath)
		if removeErr != nil && !isRemoteNotExist(removeErr) {
			return errors.Join(baseErr, fmt.Errorf("cleanup remote temp file %q: %w", tempPath, removeErr))
		}
		return baseErr
	}

	if err := copyWithProgress(ctx, source, target, item.size, item.localPath, state); err != nil {
		_ = target.Close()
		return cleanupTemp(err)
	}
	if err := target.Close(); err != nil {
		return cleanupTemp(fmt.Errorf("close remote file %q: %w", tempPath, err))
	}
	if err := f.client.Chtimes(tempPath, item.modTime, item.modTime); err != nil {
		return cleanupTemp(fmt.Errorf("set remote times for %q: %w", tempPath, err))
	}
	if err := f.replaceFile(tempPath, item.remotePath); err != nil {
		return cleanupTemp(err)
	}

	return nil
}

func (f *RemoteFS) downloadFileAtomically(ctx context.Context, item transferItem, state *transferState) (err error) {
	if err := os.MkdirAll(filepath.Dir(item.localPath), 0o755); err != nil {
		return fmt.Errorf("create local parent directory for %q: %w", item.localPath, err)
	}

	source, _, err := f.clientReader(item.remotePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	tempPath := tempLocalPath(item.localPath)
	_ = os.Remove(tempPath)

	target, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, item.mode.Perm())
	if err != nil {
		return fmt.Errorf("open local temp file %q for write: %w", tempPath, err)
	}

	cleanupTemp := func(baseErr error) error {
		removeErr := os.Remove(tempPath)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return errors.Join(baseErr, fmt.Errorf("cleanup local temp file %q: %w", tempPath, removeErr))
		}
		return baseErr
	}

	if err := copyWithProgress(ctx, source, target, item.size, item.remotePath, state); err != nil {
		_ = target.Close()
		return cleanupTemp(err)
	}
	if err := target.Close(); err != nil {
		return cleanupTemp(fmt.Errorf("close local file %q: %w", tempPath, err))
	}
	if err := os.Chtimes(tempPath, item.modTime, item.modTime); err != nil {
		return cleanupTemp(fmt.Errorf("set local times for %q: %w", tempPath, err))
	}
	if err := os.Rename(tempPath, item.localPath); err != nil {
		return cleanupTemp(fmt.Errorf("replace local file %q with %q: %w", item.localPath, tempPath, err))
	}

	return nil
}

func buildUploadPlan(localPath, remotePath string) (transferPlan, error) {
	entry, err := StatLocal(localPath)
	if err != nil {
		return transferPlan{}, err
	}
	if entry.Mode&os.ModeSymlink != 0 {
		return transferPlan{}, fmt.Errorf("symlink transfers are not supported yet: %s", localPath)
	}

	if !entry.IsDir {
		return transferPlan{
			items: []transferItem{{
				localPath:  localPath,
				remotePath: remotePath,
				mode:       entry.Mode,
				modTime:    entry.ModTime,
				size:       entry.Size,
			}},
			bytesTotal: entry.Size,
			filesTotal: 1,
		}, nil
	}

	items := []transferItem{{
		localPath:  localPath,
		remotePath: remotePath,
		mode:       entry.Mode,
		modTime:    entry.ModTime,
		dir:        true,
	}}

	var bytesTotal int64
	filesTotal := 0

	err = filepath.WalkDir(localPath, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == localPath {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink transfers are not supported yet: %s", current)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(localPath, current)
		if err != nil {
			return err
		}
		remoteItemPath := path.Join(remotePath, filepath.ToSlash(rel))
		item := transferItem{
			localPath:  current,
			remotePath: remoteItemPath,
			mode:       info.Mode(),
			modTime:    info.ModTime(),
			size:       info.Size(),
			dir:        info.IsDir(),
		}
		items = append(items, item)
		if !item.dir {
			bytesTotal += item.size
			filesTotal++
		}
		return nil
	})
	if err != nil {
		return transferPlan{}, fmt.Errorf("build upload plan for %q: %w", localPath, err)
	}

	return transferPlan{
		items:      items,
		bytesTotal: bytesTotal,
		filesTotal: filesTotal,
	}, nil
}

func (f *RemoteFS) buildDownloadPlan(ctx context.Context, remotePath, localPath string) (transferPlan, error) {
	entry, err := f.Stat(remotePath)
	if err != nil {
		return transferPlan{}, err
	}
	return buildDownloadPlanWithReader(ctx, entry, localPath, func(ctx context.Context, remoteBase string) ([]Entry, error) {
		children, err := f.client.ReadDirContext(ctx, remoteBase)
		if err != nil {
			return nil, fmt.Errorf("read remote directory %q: %w", remoteBase, err)
		}

		entries := make([]Entry, 0, len(children))
		for _, child := range children {
			entries = append(entries, EntryFromFileInfo(path.Join(remoteBase, child.Name()), child))
		}
		SortEntries(entries)
		return entries, nil
	})
}

type remoteDirReader func(context.Context, string) ([]Entry, error)

func buildDownloadPlanWithReader(ctx context.Context, root Entry, localPath string, readDir remoteDirReader) (transferPlan, error) {
	remotePath := root.Path
	if root.Mode&os.ModeSymlink != 0 {
		return transferPlan{}, fmt.Errorf("symlink transfers are not supported yet: %s", remotePath)
	}

	if !root.IsDir {
		return transferPlan{
			items: []transferItem{{
				localPath:  localPath,
				remotePath: remotePath,
				mode:       root.Mode,
				modTime:    root.ModTime,
				size:       root.Size,
			}},
			bytesTotal: root.Size,
			filesTotal: 1,
		}, nil
	}

	items := []transferItem{{
		localPath:  localPath,
		remotePath: remotePath,
		mode:       root.Mode,
		modTime:    root.ModTime,
		dir:        true,
	}}

	var bytesTotal int64
	filesTotal := 0

	var walk func(remoteBase, localBase string) error
	walk = func(remoteBase, localBase string) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		entries, err := readDir(ctx, remoteBase)
		if err != nil {
			return err
		}

		for _, child := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if child.Mode&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink transfers are not supported yet: %s", child.Path)
			}

			localChild := filepath.Join(localBase, child.Name)
			items = append(items, transferItem{
				localPath:  localChild,
				remotePath: child.Path,
				mode:       child.Mode,
				modTime:    child.ModTime,
				size:       child.Size,
				dir:        child.IsDir,
			})
			if child.IsDir {
				if err := walk(child.Path, localChild); err != nil {
					return err
				}
				continue
			}

			bytesTotal += child.Size
			filesTotal++
		}
		return nil
	}

	if err := walk(remotePath, localPath); err != nil {
		return transferPlan{}, fmt.Errorf("build download plan for %q: %w", remotePath, err)
	}

	return transferPlan{
		items:      items,
		bytesTotal: bytesTotal,
		filesTotal: filesTotal,
	}, nil
}

type transferState struct {
	progress  TransferProgress
	bytesDone int64
	itemsDone int
	emit      func(TransferProgress)
}

func (f *RemoteFS) runTransfer(ctx context.Context, direction string, plan transferPlan, progress func(TransferProgress), runItem func(item transferItem, state *transferState) error) error {
	state := &transferState{
		progress: TransferProgress{
			Direction:  direction,
			BytesTotal: plan.bytesTotal,
			ItemsTotal: plan.filesTotal,
		},
		emit: progress,
	}
	state.emitProgress()

	for _, item := range plan.items {
		if err := ctx.Err(); err != nil {
			return err
		}

		if !item.dir {
			state.progress.CurrentPath = pathLabel(direction, item)
			state.progress.CurrentBytes = 0
			state.progress.CurrentTotal = item.size
			state.emitProgress()
		}

		if err := runItem(item, state); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			if item.dir {
				return fmt.Errorf("%s directory %q: %w", direction, pathLabel(direction, item), err)
			}
			return fmt.Errorf("%s file %q: %w", direction, pathLabel(direction, item), err)
		}

		if !item.dir {
			state.itemsDone++
			state.progress.ItemsDone = state.itemsDone
			state.progress.CurrentBytes = item.size
			state.progress.CurrentTotal = item.size
			state.emitProgress()
		}
	}

	state.progress.Completed = true
	state.progress.CurrentPath = ""
	state.progress.CurrentBytes = 0
	state.progress.CurrentTotal = 0
	state.emitProgress()
	return nil
}

func copyWithProgress(ctx context.Context, source io.Reader, target io.Writer, size int64, currentPath string, state *transferState) error {
	buffer := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		n, readErr := source.Read(buffer)
		if n > 0 {
			written, writeErr := target.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}

			state.bytesDone += int64(written)
			state.progress.CurrentPath = currentPath
			state.progress.BytesDone = state.bytesDone
			state.progress.CurrentBytes += int64(written)
			state.progress.CurrentTotal = size
			state.emitProgress()
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func (s *transferState) emitProgress() {
	if s.emit == nil {
		return
	}
	s.emit(s.progress)
}

func pathLabel(direction string, item transferItem) string {
	if direction == "download" {
		return item.remotePath
	}
	return item.localPath
}

func tempRemotePath(targetPath string) string {
	base := path.Base(targetPath)
	return path.Join(path.Dir(targetPath), fmt.Sprintf(".%s.vpsm-part-%d", base, time.Now().UnixNano()))
}

func tempLocalPath(targetPath string) string {
	base := filepath.Base(targetPath)
	return filepath.Join(filepath.Dir(targetPath), fmt.Sprintf(".%s.vpsm-part-%d", base, time.Now().UnixNano()))
}
