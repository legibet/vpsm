package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"vpsm/internal/filexfer"
	"vpsm/internal/model"
	"vpsm/internal/secret"
	"vpsm/internal/sshutil"
	"vpsm/internal/ui"
)

type hostGetter interface {
	Get(ctx context.Context, alias string) (model.Host, error)
}

type metadataTracker interface {
	MarkConnected(ctx context.Context, alias string) error
}

type Service struct {
	hosts    hostGetter
	metadata metadataTracker
}

func NewService(hosts hostGetter, metadata metadataTracker) Service {
	return Service{
		hosts:    hosts,
		metadata: metadata,
	}
}

func (s Service) Connect(ctx context.Context, alias string) error {
	alias = strings.TrimSpace(alias)
	host, err := s.hosts.Get(ctx, alias)
	if err != nil {
		return err
	}

	password, hasPassword, err := secret.GetPasswordIfExists(alias)
	if err != nil {
		return err
	}
	if hasPassword && strings.TrimSpace(password) != "" {
		if err := sshutil.EnsureHostKeyAcceptedContext(ctx, host, os.Stdin, os.Stdout, os.Stderr); err != nil {
			return err
		}
	}

	cmd, err := sshutil.BuildCommandWithPasswordContext(ctx, host, password)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if isUserInterruptError(err) {
			return nil
		}
		return fmt.Errorf("run ssh for %q: %w", alias, err)
	}

	return s.metadata.MarkConnected(ctx, alias)
}

func (s Service) RunFiles(ctx context.Context, alias string) error {
	alias = strings.TrimSpace(alias)
	host, err := s.hosts.Get(ctx, alias)
	if err != nil {
		return err
	}

	password, hasPassword, err := secret.GetPasswordIfExists(alias)
	if err != nil {
		return err
	}

	if err := sshutil.EnsureHostKeyAcceptedContext(ctx, host, os.Stdin, os.Stdout, os.Stderr); err != nil {
		return err
	}

	remoteFS, err := filexfer.OpenRemoteFSContext(ctx, host, password)
	if err != nil {
		if !hasPassword || strings.TrimSpace(password) == "" {
			return fmt.Errorf("%w; files mode needs a stored password or non-interactive key auth", err)
		}
		return err
	}
	defer func() {
		_ = remoteFS.Close()
	}()

	remoteDir, err := remoteFS.Getwd()
	if err != nil {
		return err
	}

	if err := s.metadata.MarkConnected(ctx, alias); err != nil {
		return err
	}

	return ui.RunFileBrowser(ui.FileBrowserOptions{
		Context:   ctx,
		Alias:     alias,
		RemoteDir: remoteDir,
		Remote:    remoteFS,
	})
}

func (s Service) BuildFilesCommand(alias string) (*exec.Cmd, error) {
	bin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve vpsm binary: %w", err)
	}

	return exec.Command(bin, "files", alias), nil
}

func isUserInterruptError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 130 {
		return true
	}

	return false
}
