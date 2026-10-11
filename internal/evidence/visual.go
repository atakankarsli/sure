package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/atakankarsli/sure/internal/gitview"
)

// The visual manifest is reserved for v0: Sure parses and checks it, but no
// configured provider judges images, so a valid artifact still yields
// unknown/unsupported_evidence_kind.
const (
	VisualSchema        = "sure.visual-manifest/v1"
	MaxVisualManifest   = 1 << 20
	MaxVisualArtifact   = 15 << 20
	VisualMissing       = "missing_visual_evidence"
	VisualStale         = "stale_evidence"
	VisualHashMismatch  = "artifact_hash_mismatch"
	VisualUnsafe        = "unsafe_artifact_path"
	VisualUnsupported   = "unsupported_evidence_kind"
	VisualManifestError = "invalid_visual_manifest"
)

type VisualManifest struct {
	SchemaVersion string        `json:"schema_version"`
	Entries       []VisualEntry `json:"entries"`
}

type VisualEntry struct {
	Check          string   `json:"check"`
	Scenario       string   `json:"scenario"`
	Artifact       string   `json:"artifact"`
	ArtifactSHA256 string   `json:"artifact_sha256"`
	SourceDigest   string   `json:"source_digest"`
	Producer       Producer `json:"producer"`
	Viewport       Viewport `json:"viewport"`
	Locale         string   `json:"locale"`
	Theme          string   `json:"theme"`
	TextSize       string   `json:"text_size"`
}

type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ParseVisualManifest(data []byte) (VisualManifest, error) {
	var m VisualManifest
	if len(data) > MaxVisualManifest {
		return m, errors.New("visual manifest is too large")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("visual manifest: %w", err)
	}
	if dec.More() {
		return m, errors.New("visual manifest: trailing data")
	}
	if m.SchemaVersion != VisualSchema {
		return m, fmt.Errorf("visual manifest: schema_version must be %q", VisualSchema)
	}
	for i, e := range m.Entries {
		switch {
		case e.Check == "" || e.Scenario == "":
			return m, fmt.Errorf("visual manifest entry %d: check and scenario are required", i)
		case !hex64.MatchString(e.ArtifactSHA256) || !hex64.MatchString(e.SourceDigest):
			return m, fmt.Errorf("visual manifest entry %d: artifact_sha256 and source_digest must be sha256 hex", i)
		case e.Producer.Name == "" || e.Producer.Version == "":
			return m, fmt.Errorf("visual manifest entry %d: producer name and version are required", i)
		case e.Viewport.Width <= 0 || e.Viewport.Height <= 0:
			return m, fmt.Errorf("visual manifest entry %d: viewport must be positive", i)
		}
		if _, err := gitview.CleanRel(e.Artifact); err != nil {
			return m, fmt.Errorf("visual manifest entry %d: %w", i, err)
		}
	}
	return m, nil
}

// AssessVisual decides what a visual check can claim. With no image-capable
// provider the best outcome is unsupported_evidence_kind; it is never
// supported.
func AssessVisual(ctx context.Context, m *VisualManifest, checkID, sourceDigest string, src gitview.Source) (string, *VisualEntry) {
	if m == nil {
		return VisualMissing, nil
	}
	for i := range m.Entries {
		e := &m.Entries[i]
		if e.Check != checkID {
			continue
		}
		if e.SourceDigest != sourceDigest {
			return VisualStale, e
		}
		b, err := src.Read(ctx, e.Artifact, MaxVisualArtifact)
		if err != nil {
			if errors.Is(err, gitview.ErrUnsafe) || errors.Is(err, gitview.ErrSymlink) {
				return VisualUnsafe, e
			}
			return VisualMissing, e
		}
		if Hash(b) != e.ArtifactSHA256 {
			return VisualHashMismatch, e
		}
		return VisualUnsupported, e
	}
	return VisualMissing, nil
}
