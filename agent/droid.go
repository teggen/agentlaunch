package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// droidMarker owns our entry in droid's settings: displayName is what droid
// derives selection IDs from, and it is deliberately dash-safe. Apply
// replaces marker-owned entries and never touches others.
const droidMarker = "openrouter-launch"

// Droid launches Factory's droid via the ConfigWriter escape hatch — the
// ONE sanctioned agent-owned write (Landmine 6 as amended). Factory
// documents OpenRouter BYOK, but the only declaration surface is a
// .factory settings file: no env var, no flag, no inline config (owner
// decision at spec review: ConfigWriter, not unsupported). Apply writes a
// single marker-owned customModels entry into ~/.factory/settings.local.json
// (the merge-friendly local layer, never settings.json) with
// apiKey "${OPENROUTER_API_KEY}" — env interpolation, so the key never
// touches disk — and points the default-model key at it; restore puts both
// back. Model selection lives in the file, NOT on argv: the entry's
// index-derived custom: ID is only knowable at Apply time, and Command is
// pure. Requires a Factory account even for BYOK. Doc-verified on 0.190.0
// (2026-08-09); see .superpowers/sdd/2026-08-09-tier-2-research/droid.md.
type Droid struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (d *Droid) Name() string        { return "droid" }
func (d *Droid) DisplayName() string { return "Factory Droid" }

func (d *Droid) lookPath(file string) (string, error) {
	if d.LookPath != nil {
		return d.LookPath(file)
	}
	return exec.LookPath(file)
}

func droidSettingsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".factory", "settings.local.json"), nil
}

// Command builds the droid invocation: passthrough only, no -m (see the
// type comment), key in env for the ${OPENROUTER_API_KEY} interpolation.
func (d *Droid) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("droid: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("droid", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	path, err := d.lookPath("droid")
	if err != nil {
		return Command{}, fmt.Errorf("droid binary not found: %w", err)
	}
	return Command{
		Path: path,
		Args: append([]string(nil), req.ExtraArgs...),
		Env:  []string{"OPENROUTER_API_KEY=" + req.APIKey},
	}, nil
}

// Apply upserts the marker-owned model entry and default-model key, and
// returns the restore that undoes exactly that. An unparseable settings
// file is a hard error — never clobber what we cannot understand.
func (d *Droid) Apply(req Request) (func() error, error) {
	path, err := droidSettingsFile()
	if err != nil {
		return nil, err
	}
	settings, existed, err := readDroidSettingsFile(path)
	if err != nil {
		return nil, err
	}

	priorModel, hadModel := settings["model"]

	kept := foreignDroidModels(settings)
	entry := map[string]any{
		"displayName":     droidMarker,
		"provider":        "generic-chat-completion-api",
		"baseUrl":         openrouter.DefaultBaseURL,
		"model":           req.Model.ID,
		"apiKey":          "${OPENROUTER_API_KEY}",
		"maxOutputTokens": 64000,
	}
	settings["customModels"] = append(kept, entry)
	settings["model"] = fmt.Sprintf("custom:%s-%d", droidMarker, len(kept))

	if err := writeDroidSettingsFile(path, settings); err != nil {
		return nil, err
	}

	restore := func() error {
		settings, _, err := readDroidSettingsFile(path)
		if err != nil {
			return err
		}
		kept := foreignDroidModels(settings)
		if len(kept) == 0 {
			delete(settings, "customModels")
		} else {
			settings["customModels"] = kept
		}
		if hadModel {
			settings["model"] = priorModel
		} else {
			delete(settings, "model")
		}
		if !existed && len(settings) == 0 {
			return os.Remove(path)
		}
		return writeDroidSettingsFile(path, settings)
	}
	return restore, nil
}

// readDroidSettingsFile loads the settings map; a missing file is an empty
// map with existed=false.
func readDroidSettingsFile(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false, fmt.Errorf("droid: %s is not valid JSON (%w); refusing to modify it", path, err)
	}
	return m, true, nil
}

// foreignDroidModels returns customModels entries we do not own, in their
// original order. A user editing the file mid-session keeps their entries.
func foreignDroidModels(settings map[string]any) []any {
	models, _ := settings["customModels"].([]any)
	var kept []any
	for _, item := range models {
		if entry, ok := item.(map[string]any); ok && entry["displayName"] == droidMarker {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// writeDroidSettingsFile writes atomically: temp file in the same dir, then
// rename (the Landmine 9 shape; 0644 because no secret is inside — the
// apiKey field holds the literal interpolation string).
func writeDroidSettingsFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// CheckInstalled reports whether the droid binary can be found. The
// standalone installer puts it in ~/.local/bin, which the installer adds to
// PATH; there is no reliable secondary location.
func (d *Droid) CheckInstalled() bool {
	_, err := d.lookPath("droid")
	return err == nil
}

// InstallHint tells the user how to install droid. Printed, never run.
// Droid requires a Factory account even on the BYOK-only tier.
func (d *Droid) InstallHint() string {
	return "Install droid: curl -fsSL https://app.factory.ai/cli | sh (requires a Factory account, even for BYOK)"
}
