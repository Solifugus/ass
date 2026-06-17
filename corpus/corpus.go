package corpus

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Expected captures the expected outcomes for a corpus item (from meta.yaml).
type Expected struct {
	Parse   string `yaml:"parse"`   // pass | fail
	Execute string `yaml:"execute"` // pass | fail | skip
	Output  string `yaml:"output"`  // verified | unverified | none
}

// Item is a single compatibility-test item loaded from disk.
type Item struct {
	ID       string   `yaml:"id"`
	Source   string   `yaml:"source"`
	License  string   `yaml:"license"`
	Features []string `yaml:"features"`
	Expected Expected `yaml:"expected"`
	Priority int      `yaml:"priority"`
	Notes    string   `yaml:"notes"`

	// Populated from the item directory (not from meta.yaml).
	Dir            string `yaml:"-"`
	Input          string `yaml:"-"` // contents of input.sas
	ExpectedOutput string `yaml:"-"` // contents of expected_output.txt, if present
	ExpectedLog    string `yaml:"-"` // contents of expected_log.txt, if present
}

// Load reads every corpus item under dir. An item is any subdirectory that
// contains both meta.yaml and input.sas. Items are returned sorted by ID.
func Load(dir string) ([]Item, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		itemDir := filepath.Join(dir, e.Name())
		metaPath := filepath.Join(itemDir, "meta.yaml")
		inputPath := filepath.Join(itemDir, "input.sas")
		if !fileExists(metaPath) || !fileExists(inputPath) {
			continue
		}
		item, err := loadItem(itemDir, metaPath, inputPath)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func loadItem(dir, metaPath, inputPath string) (Item, error) {
	var item Item
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return item, err
	}
	if err := yaml.Unmarshal(metaBytes, &item); err != nil {
		return item, err
	}
	input, err := os.ReadFile(inputPath)
	if err != nil {
		return item, err
	}
	item.Dir = dir
	item.Input = string(input)
	if b, err := os.ReadFile(filepath.Join(dir, "expected_output.txt")); err == nil {
		item.ExpectedOutput = string(b)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "expected_log.txt")); err == nil {
		item.ExpectedLog = string(b)
	}
	if item.ID == "" {
		item.ID = filepath.Base(dir)
	}
	return item, nil
}

// HasFeature reports whether the item is tagged with the given feature.
func (it Item) HasFeature(tag string) bool {
	for _, f := range it.Features {
		if f == tag {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
