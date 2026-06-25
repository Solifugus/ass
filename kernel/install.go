package kernel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// kernelSpec is the kernel.json a Jupyter frontend reads to launch ASS. Jupyter
// substitutes {connection_file} with the path to the connection file it writes.
type kernelSpec struct {
	Argv        []string `json:"argv"`
	DisplayName string   `json:"display_name"`
	Language    string   `json:"language"`
}

// InstallSpec writes the ASS kernelspec into the user's Jupyter data directory
// so `jupyter notebook`/`jupyter lab` lists an "ASS (SAS)" kernel. It returns
// the directory it wrote. The argv points at the current executable, so the
// kernel that gets launched is exactly this build.
func InstallSpec() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the ass executable: %w", err)
	}
	exe, _ = filepath.Abs(exe)

	dir := filepath.Join(kernelsDir(), "ass")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	spec := kernelSpec{
		Argv:        []string{exe, "kernel", "{connection_file}"},
		DisplayName: "ASS (SAS)",
		Language:    "sas",
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "kernel.json"), data, 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

// kernelsDir returns the per-user Jupyter kernels directory, honoring the
// standard environment overrides (JUPYTER_DATA_DIR, then XDG_DATA_HOME), with
// the platform default as a fallback.
func kernelsDir() string {
	if d := os.Getenv("JUPYTER_DATA_DIR"); d != "" {
		return filepath.Join(d, "kernels")
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Jupyter", "kernels")
	case "windows":
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "jupyter", "kernels")
		}
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "jupyter", "kernels")
	}
	return filepath.Join(home, ".local", "share", "jupyter", "kernels")
}
