package app

import (
	"context"
	"fmt"

	"vpsm/internal/model"
	"vpsm/internal/sshutil"
)

type keySetupResult struct {
	IdentityFile string
}

type keySetupRunner interface {
	Setup(ctx context.Context, host model.Host, password string) (keySetupResult, error)
}

// SetupManagedHostKey ensures the selected managed host has a local key pair and
// uploads the corresponding public key to the remote server.
func (s HostService) SetupManagedHostKey(ctx context.Context, alias string) error {
	managedHost, err := s.getManagedHost(alias)
	if err != nil {
		return err
	}

	password, _, err := s.passwords.GetPasswordIfExists(managedHost.Alias)
	if err != nil {
		return err
	}

	result, err := s.keySetup.Setup(ctx, model.Host{
		Alias:        managedHost.Alias,
		DisplayName:  managedHost.DisplayName,
		HostName:     managedHost.HostName,
		User:         managedHost.User,
		Port:         managedHost.Port,
		Managed:      true,
		IdentityFile: managedHost.IdentityFile,
	}, password)
	if err != nil {
		return err
	}

	if result.IdentityFile == "" || result.IdentityFile == managedHost.IdentityFile {
		return nil
	}

	managedHost.IdentityFile = result.IdentityFile
	if err := s.managedHosts.Upsert(managedHost); err != nil {
		return fmt.Errorf("update identity file for %q: %w", managedHost.Alias, err)
	}

	return nil
}

type systemKeySetupRunner struct{}

func (systemKeySetupRunner) Setup(ctx context.Context, host model.Host, password string) (keySetupResult, error) {
	plan, err := sshutil.PlanKeySetup(host.Alias, host.IdentityFile)
	if err != nil {
		return keySetupResult{}, err
	}

	if err := sshutil.EnsureKeyPairContext(ctx, plan, "vpsm:"+host.Alias); err != nil {
		return keySetupResult{}, err
	}

	publicKey, err := sshutil.ReadPublicKey(plan)
	if err != nil {
		return keySetupResult{}, err
	}

	target := host
	target.IdentityFile = plan.IdentityFile
	if err := sshutil.EnsureHostKeyAcceptedContext(ctx, target, nil, nil, nil); err != nil {
		return keySetupResult{}, err
	}

	if err := sshutil.InstallPublicKeyContext(ctx, target, password, publicKey); err != nil {
		return keySetupResult{}, err
	}

	return keySetupResult{IdentityFile: plan.IdentityFile}, nil
}
