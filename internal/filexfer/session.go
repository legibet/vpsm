package filexfer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/pkg/sftp"

	"vpsm/internal/model"
	"vpsm/internal/sshutil"
)

type RemoteFS struct {
	host   model.Host
	cmd    *exec.Cmd
	client *sftp.Client
	stderr bytes.Buffer
}

func OpenRemoteFSContext(ctx context.Context, host model.Host, password string) (*RemoteFS, error) {
	cmd, err := sshutil.BuildSubsystemCommandWithPasswordContext(ctx, host, password, "sftp")
	if err != nil {
		return nil, err
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open sftp stdin for %q: %w", host.Alias, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open sftp stdout for %q: %w", host.Alias, err)
	}

	fs := &RemoteFS{
		host: host,
		cmd:  cmd,
	}
	cmd.Stderr = &fs.stderr

	if err := cmd.Start(); err != nil {
		return nil, fs.wrapCommandError("start sftp subsystem", err)
	}

	client, err := sftp.NewClientPipe(stdout, stdin,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
		sftp.MaxConcurrentRequestsPerFile(64),
	)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fs.wrapCommandError("open sftp client", err)
	}

	fs.client = client
	return fs, nil
}

func (f *RemoteFS) Close() error {
	var errs []error

	if f.client != nil {
		if err := f.client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close sftp client for %q: %w", f.host.Alias, err))
		}
	}

	if f.cmd != nil {
		if err := f.cmd.Wait(); err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, f.wrapCommandError("wait for sftp subsystem", err))
		}
	}

	return errors.Join(errs...)
}

func (f *RemoteFS) Getwd() (string, error) {
	wd, err := f.client.Getwd()
	if err != nil {
		return "", fmt.Errorf("read remote working directory for %q: %w", f.host.Alias, err)
	}
	return wd, nil
}

func (f *RemoteFS) ReadDir(ctx context.Context, dirPath string, showHidden bool) ([]Entry, error) {
	items, err := f.client.ReadDirContext(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("read remote directory %q: %w", dirPath, err)
	}

	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		entry := EntryFromFileInfo(path.Join(dirPath, item.Name()), item)
		if !showHidden && entry.IsHidden {
			continue
		}
		entries = append(entries, entry)
	}

	SortEntries(entries)
	return entries, nil
}

func (f *RemoteFS) Stat(fullPath string) (Entry, error) {
	info, err := f.client.Lstat(fullPath)
	if err != nil {
		return Entry{}, fmt.Errorf("stat remote path %q: %w", fullPath, err)
	}
	return EntryFromFileInfo(fullPath, info), nil
}

func (f *RemoteFS) Exists(fullPath string) (bool, error) {
	_, err := f.client.Lstat(fullPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if statusErr, ok := err.(*sftp.StatusError); ok && statusErr.Code == 2 {
		return false, nil
	}
	return false, fmt.Errorf("stat remote path %q: %w", fullPath, err)
}

func (f *RemoteFS) Mkdir(fullPath string) error {
	if err := f.client.Mkdir(fullPath); err != nil {
		return fmt.Errorf("create remote directory %q: %w", fullPath, err)
	}
	return nil
}

func (f *RemoteFS) Rename(oldPath string, newPath string) error {
	var err error
	if _, ok := f.client.HasExtension("posix-rename@openssh.com"); ok {
		err = f.client.PosixRename(oldPath, newPath)
	} else {
		err = f.client.Rename(oldPath, newPath)
	}
	if err != nil {
		return fmt.Errorf("rename remote path %q to %q: %w", oldPath, newPath, err)
	}
	return nil
}

func (f *RemoteFS) replaceFile(oldPath string, newPath string) error {
	if _, ok := f.client.HasExtension("posix-rename@openssh.com"); ok {
		if err := f.client.PosixRename(oldPath, newPath); err != nil {
			return fmt.Errorf("replace remote file %q with %q: %w", newPath, oldPath, err)
		}
		return nil
	}

	if err := f.client.Remove(newPath); err != nil && !isRemoteNotExist(err) {
		return fmt.Errorf("remove existing remote file %q: %w", newPath, err)
	}
	if err := f.client.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("replace remote file %q with %q: %w", newPath, oldPath, err)
	}
	return nil
}

func (f *RemoteFS) Remove(fullPath string) error {
	if err := f.client.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("remove remote path %q: %w", fullPath, err)
	}
	return nil
}

func (f *RemoteFS) RemoteJoin(parts ...string) string {
	return path.Join(parts...)
}

func (f *RemoteFS) wrapCommandError(action string, err error) error {
	message := strings.TrimSpace(f.stderr.String())
	if message == "" {
		return fmt.Errorf("%s for %q: %w", action, f.host.Alias, err)
	}
	return fmt.Errorf("%s for %q: %w: %s", action, f.host.Alias, err, message)
}

func (f *RemoteFS) clientReader(path string) (io.ReadCloser, Entry, error) {
	entry, err := f.Stat(path)
	if err != nil {
		return nil, Entry{}, err
	}
	file, err := f.client.Open(path)
	if err != nil {
		return nil, Entry{}, fmt.Errorf("open remote file %q: %w", path, err)
	}
	return file, entry, nil
}

func (f *RemoteFS) clientWriter(path string, mode os.FileMode) (io.WriteCloser, error) {
	file, err := f.client.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return nil, fmt.Errorf("open remote file %q for write: %w", path, err)
	}
	if mode.Perm() != 0 {
		if err := f.client.Chmod(path, mode.Perm()); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("chmod remote file %q: %w", path, err)
		}
	}
	return file, nil
}

func isRemoteNotExist(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	statusErr, ok := err.(*sftp.StatusError)
	return ok && statusErr.Code == 2
}
