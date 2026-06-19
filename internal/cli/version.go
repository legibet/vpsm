package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func currentVersionInfo() versionInfo {
	return versionInfo{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}

func (a App) runVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Print version as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	return writeVersion(os.Stdout, currentVersionInfo(), jsonOutput)
}

func writeVersion(w io.Writer, info versionInfo, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	}

	_, err := fmt.Fprintf(w, "vpsm %s\ncommit: %s\nbuilt: %s\n", info.Version, info.Commit, info.Date)
	return err
}
