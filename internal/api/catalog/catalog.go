package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
)

//go:embed data hints
var dataFS embed.FS

// CompositeField describes a field within a composite component's config that itself
// contains one or more nested ComponentSpecs (inputs, processors, or outputs).
// Field="" means the config value itself is the array (Pattern B: for_each, fallback).
type CompositeField struct {
	Field string `json:"field"` // field name in config; "" = config is directly the array
	Kind  string `json:"kind"`  // "inputs" | "processors" | "outputs"
	Multi bool   `json:"multi"` // true = array, false = single ComponentSpec
}

// Component describes one Redpanda Connect component available in the catalog.
// BodyKind values: "object" (RJSF form), "scalar" (textarea), "composite" (nested editors).
type Component struct {
	Name            string           `json:"name"`
	Category        string           `json:"category"`
	Status          string           `json:"status"`
	Summary         string           `json:"summary"`
	BodyKind        string           `json:"bodyKind"`
	ReplicaSafety   string           `json:"replicaSafety"`
	ConfigSchema    json.RawMessage  `json:"configSchema"`
	CompositeFields []CompositeField `json:"compositeFields,omitempty"`
}

// componentHint carries hand-curated UI overlay fields that the build-time
// catalog generator cannot derive from Benthos ConfigSpec metadata.
// Overlay files live under hints/<category>/<name>.json and are optional.
type componentHint struct {
	BodyKind        string           `json:"bodyKind"`
	ReplicaSafety   string           `json:"replicaSafety"`
	CompositeFields []CompositeField `json:"compositeFields,omitempty"`
}

// Catalog holds the loaded component entries indexed for fast lookup.
type Catalog struct {
	byKey map[string]*Component // key: "<category>/<name>"
	all   []*Component
}

// loadHints walks the embedded hints/ tree and returns a map keyed by "<category>/<name>".
// Missing hints/ directory or empty subtrees are not errors — hints are optional overlays.
func loadHints() (map[string]componentHint, error) {
	out := map[string]componentHint{}
	err := fs.WalkDir(dataFS, "hints", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Missing hints/ root is fine — no overlays defined.
			if p == "hints" {
				return fs.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil
		}
		b, err := dataFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		var h componentHint
		if err := json.Unmarshal(b, &h); err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
		category := path.Base(path.Dir(p))
		name := strings.TrimSuffix(path.Base(p), ".json")
		out[category+"/"+name] = h
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Load parses all embedded JSON files and returns a populated Catalog.
// Schema-level fields (name, category, status, summary, configSchema) come from
// data/<category>/<name>.json; UI overlay fields (bodyKind, replicaSafety,
// compositeFields) are filled from hints/<category>/<name>.json when the data
// file leaves them unset, with sensible defaults as a final fallback.
func Load() (*Catalog, error) {
	hints, err := loadHints()
	if err != nil {
		return nil, err
	}
	c := &Catalog{byKey: map[string]*Component{}}
	err = fs.WalkDir(dataFS, "data", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") {
			return err
		}
		b, err := dataFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		var comp Component
		if err := json.Unmarshal(b, &comp); err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
		// Verify the file's directory matches the component's category.
		wantCategory := path.Base(path.Dir(p))
		if comp.Category != wantCategory {
			return fmt.Errorf("%s: category %q != directory %q", p, comp.Category, wantCategory)
		}
		// Overlay hand-curated hints only where the data file is silent. This keeps
		// the merge a no-op while data files still carry inline hints (pre-migration),
		// and becomes the active source once data files are slimmed down.
		if h, ok := hints[comp.Category+"/"+comp.Name]; ok {
			if comp.BodyKind == "" {
				comp.BodyKind = h.BodyKind
			}
			if comp.ReplicaSafety == "" {
				comp.ReplicaSafety = h.ReplicaSafety
			}
			if len(comp.CompositeFields) == 0 {
				comp.CompositeFields = h.CompositeFields
			}
		}
		if comp.BodyKind == "" {
			comp.BodyKind = "object"
		}
		if comp.ReplicaSafety == "" {
			comp.ReplicaSafety = "unknown"
		}
		c.byKey[comp.Category+"/"+comp.Name] = &comp
		c.all = append(c.all, &comp)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns a component by category and name, or false if not found.
func (c *Catalog) Get(category, name string) (*Component, bool) {
	comp, ok := c.byKey[category+"/"+name]
	return comp, ok
}

// All returns a copy of all components in the catalog.
func (c *Catalog) All() []*Component {
	out := make([]*Component, len(c.all))
	copy(out, c.all)
	return out
}

var (
	once    sync.Once
	cached  *Catalog
	loadErr error
)

// Default returns the singleton catalog, loading it on first call.
func Default() (*Catalog, error) {
	once.Do(func() { cached, loadErr = Load() })
	return cached, loadErr
}
