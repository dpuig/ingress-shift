// Command krew-manifest regenerates krew.yaml for a release: it computes the
// real sha256 of each platform archive produced by .github/workflows/release.yml
// and writes the version/uri/sha256 fields, replacing the placeholder values
// that ship in the repo before the first real release.
//
// Usage: go run ./tools/krew-manifest <version> <dist-dir> <krew.yaml path>
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

const repo = "dpuig/ingress-shift"
const binaryName = "ingress-shift-analyzer"

type matchLabels struct {
	OS   string `yaml:"os"`
	Arch string `yaml:"arch"`
}

type selector struct {
	MatchLabels matchLabels `yaml:"matchLabels"`
}

type platform struct {
	Selector selector `yaml:"selector"`
	URI      string   `yaml:"uri"`
	Sha256   string   `yaml:"sha256"`
	Bin      string   `yaml:"bin"`
}

type link struct {
	Text string `yaml:"text"`
	URL  string `yaml:"url"`
}

type spec struct {
	Version          string     `yaml:"version"`
	Platforms        []platform `yaml:"platforms"`
	ShortDescription string     `yaml:"shortDescription"`
	Description      string     `yaml:"description"`
	Homepage         string     `yaml:"homepage"`
	Links            []link     `yaml:"links"`
}

type metadata struct {
	Name string `yaml:"name"`
}

type manifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   metadata `yaml:"metadata"`
	Spec       spec     `yaml:"spec"`
}

// target mirrors the build matrix in .github/workflows/release.yml.
type target struct {
	os, arch, ext, archiveExt string
}

var targets = []target{
	{os: "linux", arch: "amd64", ext: "", archiveExt: "tar.gz"},
	{os: "linux", arch: "arm64", ext: "", archiveExt: "tar.gz"},
	{os: "darwin", arch: "amd64", ext: "", archiveExt: "tar.gz"},
	{os: "darwin", arch: "arm64", ext: "", archiveExt: "tar.gz"},
	{os: "windows", arch: "amd64", ext: ".exe", archiveExt: "zip"},
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: krew-manifest <version> <dist-dir> <krew.yaml path>")
		os.Exit(2)
	}

	version, distDir, krewPath := os.Args[1], os.Args[2], os.Args[3]

	platforms := make([]platform, 0, len(targets))
	for _, tgt := range targets {
		archiveName := fmt.Sprintf("%s-%s-%s.%s", binaryName, tgt.os, tgt.arch, tgt.archiveExt)
		archivePath := filepath.Join(distDir, archiveName)

		sum, err := sha256File(archivePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to hash %s: %v\n", archivePath, err)
			os.Exit(1)
		}

		platforms = append(platforms, platform{
			Selector: selector{MatchLabels: matchLabels{OS: tgt.os, Arch: tgt.arch}},
			URI:      fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, version, archiveName),
			Sha256:   sum,
			Bin:      binaryName + tgt.ext,
		})
	}

	m := manifest{
		APIVersion: "krew.googlecontainertools.github.com/v1alpha2",
		Kind:       "Plugin",
		Metadata:   metadata{Name: binaryName},
		Spec: spec{
			Version:          version,
			Platforms:        platforms,
			ShortDescription: "Ingress Shift Analyzer - Analyze Ingress resources for Gateway API migration",
			Description: `Ingress Shift Analyzer is a kubectl plugin that analyzes Ingress resources
and their annotations to determine migration complexity to Gateway API.

It enumerates Ingress resources across all contexts and namespaces,
maps every annotation against a maintained knowledge base,
flags classes that break naive translation, and emits a scored report
with percentage translatable, list of manual interventions with effort estimate,
and recommendation for target controller.
`,
			Homepage: fmt.Sprintf("https://github.com/%s", repo),
			Links: []link{
				{Text: "Documentation", URL: fmt.Sprintf("https://github.com/%s/blob/main/README.md", repo)},
			},
		},
	}

	out, err := yaml.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal krew manifest: %v\n", err)
		os.Exit(1)
	}

	content := "---\n" + string(out)
	if err := os.WriteFile(krewPath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", krewPath, err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s for version %s\n", krewPath, version)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
