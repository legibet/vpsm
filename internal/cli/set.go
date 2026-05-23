package cli

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"

	"vpsm/internal/hosts"
	"vpsm/internal/model"
)

var (
	errPasswordConflict       = errors.New("cannot use --password and --clear-password together")
	errPassphraseConflict     = errors.New("cannot use --passphrase and --clear-passphrase together")
	errSystemRenameViaOverlay = errors.New("system hosts cannot be renamed via overlay")
)

type setUpdatePlan struct {
	input            hosts.UpdateManagedHostInput
	effectiveAlias   string
	localForwardSet  bool
	remoteForwardSet bool
}

type optionalString struct {
	set   bool
	value string
}

func (o *optionalString) String() string {
	return o.value
}

func (o *optionalString) Set(value string) error {
	o.set = true
	o.value = value
	return nil
}

func buildSetUpdate(current model.Host, args []string) (setUpdatePlan, error) {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var newAlias optionalString
	var displayName optionalString
	var hostName optionalString
	var user optionalString
	var portValue string
	var proxyJump optionalString
	var proxyCommand optionalString
	var forwardAgent optionalString
	var localForward optionalString
	var remoteForward optionalString
	var identityFile optionalString
	var password string
	var clearPassword bool
	var passphrase string
	var clearPassphrase bool

	fs.Var(&newAlias, "alias", "New host alias")
	fs.Var(&displayName, "name", "Display name")
	fs.Var(&hostName, "host", "Host or IP")
	fs.Var(&user, "user", "SSH user")
	fs.StringVar(&portValue, "port", "", "SSH port")
	fs.Var(&proxyJump, "proxy-jump", "SSH ProxyJump value")
	fs.Var(&proxyCommand, "proxy-command", "SSH ProxyCommand value")
	fs.Var(&forwardAgent, "forward-agent", "SSH ForwardAgent value")
	fs.Var(&localForward, "local-forward", "Comma-separated SSH LocalForward values")
	fs.Var(&remoteForward, "remote-forward", "Comma-separated SSH RemoteForward values")
	fs.Var(&identityFile, "identity-file", "SSH private key path")
	fs.StringVar(&password, "password", "", "SSH password to store in keychain")
	fs.BoolVar(&clearPassword, "clear-password", false, "Delete the stored SSH password")
	fs.StringVar(&passphrase, "passphrase", "", "SSH key passphrase to store in keychain")
	fs.BoolVar(&clearPassphrase, "clear-passphrase", false, "Delete the stored SSH key passphrase")

	if err := fs.Parse(args); err != nil {
		return setUpdatePlan{}, err
	}
	if strings.TrimSpace(password) != "" && clearPassword {
		return setUpdatePlan{}, errPasswordConflict
	}
	if strings.TrimSpace(passphrase) != "" && clearPassphrase {
		return setUpdatePlan{}, errPassphraseConflict
	}

	input := hosts.UpdateManagedHostInput{
		Alias:         current.Alias,
		DisplayName:   current.DisplayName,
		HostName:      current.HostName,
		User:          current.User,
		Port:          current.Port,
		ProxyJump:     current.ProxyJump,
		ProxyCommand:  current.ProxyCommand,
		ForwardAgent:  current.ForwardAgent,
		LocalForward:  joinForwardValues(current.LocalForward),
		RemoteForward: joinForwardValues(current.RemoteForward),
		IdentityFile:  current.IdentityFile,
	}

	changed := false
	if newAlias.set {
		normalized := hosts.NormalizeAlias(newAlias.value)
		if normalized == "" {
			return setUpdatePlan{}, errors.New("alias is required")
		}
		if !current.Managed && normalized != current.Alias {
			return setUpdatePlan{}, errSystemRenameViaOverlay
		}
		if normalized != current.Alias {
			input.NewAlias = normalized
			changed = true
		}
	}
	if displayName.set {
		input.DisplayName = displayName.value
		changed = true
	}
	if hostName.set {
		input.HostName = hostName.value
		changed = true
	}
	if user.set {
		input.User = user.value
		changed = true
	}
	if strings.TrimSpace(portValue) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(portValue))
		if err != nil || parsed <= 0 {
			return setUpdatePlan{}, errors.New("port must be a positive number")
		}
		input.Port = parsed
		changed = true
	}
	if proxyJump.set {
		input.ProxyJump = proxyJump.value
		changed = true
	}
	if proxyCommand.set {
		input.ProxyCommand = proxyCommand.value
		changed = true
	}
	if forwardAgent.set {
		input.ForwardAgent = forwardAgent.value
		changed = true
	}
	if localForward.set {
		input.LocalForward = localForward.value
		changed = true
	}
	if remoteForward.set {
		input.RemoteForward = remoteForward.value
		changed = true
	}
	if identityFile.set {
		input.IdentityFile = identityFile.value
		changed = true
	}
	if strings.TrimSpace(password) != "" {
		input.Password = password
		changed = true
	}
	if clearPassword {
		input.ClearPassword = true
		changed = true
	}
	if strings.TrimSpace(passphrase) != "" {
		input.Passphrase = passphrase
		changed = true
	}
	if clearPassphrase {
		input.ClearPassphrase = true
		changed = true
	}
	if !changed {
		return setUpdatePlan{}, errors.New("no changes requested")
	}

	effectiveAlias := current.Alias
	if strings.TrimSpace(input.NewAlias) != "" {
		effectiveAlias = input.NewAlias
	}

	return setUpdatePlan{
		input:            input,
		effectiveAlias:   effectiveAlias,
		localForwardSet:  localForward.set,
		remoteForwardSet: remoteForward.set,
	}, nil
}
