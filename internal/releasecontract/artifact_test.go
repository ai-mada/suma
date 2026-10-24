package releasecontract

import (
	"strings"
	"testing"
)

func TestManifestCanonicalDigestAndLimits(t *testing.T) {
	file := func(path string) ArtifactFile {
		return ArtifactFile{Path: path, SizeBytes: 1, SHA256: strings.Repeat("a", 64)}
	}
	a := ArtifactManifest{SchemaVersion: 1, Kind: StaticSiteKind, Files: []ArtifactFile{file("index.html"), file("assets/app.js")}}
	b := a
	b.Files = []ArtifactFile{a.Files[1], a.Files[0]}
	ca, ha, err := CanonicalManifest(a)
	if err != nil {
		t.Fatal(err)
	}
	_, hb, err := CanonicalManifest(b)
	if err != nil || ha != hb || ca.Files[0].Path != "assets/app.js" {
		t.Fatalf("canonical: %v %s %s", err, ha, hb)
	}
	if a.Files[0].Path != "index.html" {
		t.Fatal("caller input was mutated")
	}
	for _, path := range []string{"../secret", "/absolute", "a\\b", "a//b", "AUX.txt", "dir/NUL", "name.", "a:b", "a%2fb", ".suma-release", "a/../b", "中.html"} {
		t.Run(path, func(t *testing.T) {
			m := a
			m.Files = []ArtifactFile{file(path)}
			if _, _, e := CanonicalManifest(m); e == nil {
				t.Fatalf("accepted %q", path)
			}
		})
	}
	for _, files := range [][]ArtifactFile{{}, {file("A.html"), file("a.html")}, {file("dir"), file("dir/file")}, {file("index.html"), {Path: "big", SizeBytes: MaxFileBytes + 1, SHA256: strings.Repeat("a", 64)}}} {
		m := a
		m.Files = files
		if _, _, e := CanonicalManifest(m); e == nil {
			t.Fatalf("accepted bad manifest %+v", files)
		}
	}
	m := a
	m.Files = make([]ArtifactFile, MaxFiles+1)
	if _, _, err := CanonicalManifest(m); err == nil {
		t.Fatal("accepted too many files")
	}
}

func TestProfileDigestBindsAdminConfiguration(t *testing.T) {
	p := CommandProfile{ID: "site-build", Kind: "build", Executable: "go", Args: []string{"run", "./cmd/build"}, ArtifactOutput: ".cache/site"}
	h, err := ProfileDigest(p)
	if err != nil || len(h) != 64 {
		t.Fatal(h, err)
	}
	p.Args[1] = "./cmd/other"
	h2, err := ProfileDigest(p)
	if err != nil || h == h2 {
		t.Fatal("profile change did not invalidate digest", err)
	}
	p.Kind = "package_test"
	p.ArtifactOutput = ""
	p.ArtifactInput = true
	p.TestEvidence = true
	if _, err = ProfileDigest(p); err != nil {
		t.Fatal(err)
	}
	p.ReadOnly = true
	if _, err = ProfileDigest(p); err == nil {
		t.Fatal("shared workspace release profile accepted")
	}
}
