package main

import (
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/cover"
	"golang.org/x/tools/go/packages"
)

func Test_Main(t *testing.T) {
	t.Parallel()

	fname := filepath.Join(t.TempDir(), "stdout")
	temp, err := os.Create(fname)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	stdout := os.Stdout
	defer func() {
		os.Stdout = stdout
	}()

	os.Stdout = temp
	main()
	os.Stdout = stdout

	if err := temp.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	outputBytes, err := os.ReadFile(fname)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}

	outputString := string(outputBytes)
	if !strings.Contains(outputString, xml.Header) {
		t.Errorf("missing XML header")
	}
	if !strings.Contains(outputString, coberturaDTDDecl) {
		t.Errorf("missing DTDDecl")
	}
}

func TestConvertParseProfilesError(t *testing.T) {
	t.Parallel()

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	defer pipe2wr.Close()

	err := convert(strings.NewReader("invalid data"), pipe2wr, &Ignore{}, "")
	if err == nil || !strings.Contains(err.Error(), "bad mode line: invalid data") {
		t.Fatalf("expected error about bad mode line, got: %v", err)
	}
}

func TestConvertOutputError(t *testing.T) {
	t.Parallel()

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	if err := pipe2rd.Close(); err != nil {
		t.Fatalf("failed to close pipe2rd: %v", err)
	}

	err := convert(strings.NewReader("mode: set"), pipe2wr, &Ignore{}, "")
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected error about closed pipe, got: %v", err)
	}
}

func TestConvertEmpty(t *testing.T) {
	t.Parallel()

	data := `mode: set`

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	defer pipe2wr.Close()

	var eg errgroup.Group
	eg.Go(func() error {
		err := convert(strings.NewReader(data), pipe2wr, &Ignore{}, "")
		if strings.Contains(err.Error(), "write XML footer") {
			return nil
		}
		return err
	})
	defer eg.Wait()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err := dec.Decode(&v)
	if err != nil {
		t.Fatalf("failed to decode XML: %v", err)
	}
	if err := pipe2rd.Close(); err != nil {
		t.Fatalf("failed to close pipe2rd: %v", err)
	}
	if err := eg.Wait(); err != nil {
		t.Fatalf("error during conversion: %v", err)
	}

	if v.XMLName.Local != "coverage" {
		t.Errorf("expected XML name 'coverage', got '%s'", v.XMLName.Local)
	}
	if len(v.Sources) != 0 {
		t.Errorf("expected no sources, got %d", len(v.Sources))
	}
	if len(v.Packages) != 0 {
		t.Errorf("expected no packages, got %d", len(v.Packages))
	}
}

func TestParseProfileNilPackages(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := cover.Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, nil, &Ignore{})
	if err == nil || !strings.Contains(err.Error(), "package required when using go modules") {
		t.Fatalf("expected error about missing package, got: %v", err)
	}
}

func TestParseProfileEmptyPackages(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := cover.Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, &packages.Package{}, &Ignore{})
	if err == nil || !strings.Contains(err.Error(), "package required when using go modules") {
		t.Fatalf("expected error about missing package, got: %v", err)
	}
}

func TestParseProfileDoesNotExist(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := cover.Profile{FileName: "does-not-exist"}

	pkg := packages.Package{
		Name:   "does-not-exist",
		Module: &packages.Module{},
	}

	err := v.parseProfile(&profile, &pkg, &Ignore{})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected error about file not existing, got: %v", err)
	}
}

func TestParseProfileNotReadable(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := cover.Profile{FileName: os.DevNull}
	err := v.parseProfile(&profile, nil, &Ignore{})
	if err == nil || !strings.Contains(err.Error(), "package required when using go modules") {
		t.Fatalf("expected error about missing package, got: %v", err)
	}
}

func TestParseProfilePermissionDenied(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("chmod is not supported by Windows")
	}

	tempFile, err := os.CreateTemp(t.TempDir(), "not-readable")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	err = tempFile.Chmod(0o000)
	if err != nil {
		t.Fatalf("failed to change file permissions: %v", err)
	}
	v := Coverage{}
	profile := cover.Profile{FileName: tempFile.Name()}
	pkg := packages.Package{
		GoFiles: []string{
			tempFile.Name(),
		},
		Module: &packages.Module{
			Path: filepath.Dir(tempFile.Name()),
		},
	}
	err = v.parseProfile(&profile, &pkg, &Ignore{})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("expected permission denied error, got: %v", err)
	}
}

func TestConvertSetMode(t *testing.T) {
	t.Parallel()

	src, err := os.Open("testdata/testdata_set.txt")
	if err != nil {
		t.Fatalf("failed to open testdata_set.txt: %v", err)
	}
	defer src.Close()

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	defer pipe2wr.Close()

	var eg errgroup.Group
	eg.Go(func() error {
		err := convert(src, pipe2wr, &Ignore{
			GeneratedFiles: true,
			Files:          regexp.MustCompile(`[\\/]func[45]\.go$`),
		}, "testdata")
		if strings.Contains(err.Error(), "write XML footer") {
			return nil
		}
		return err
	})
	defer eg.Wait()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	if err != nil {
		t.Fatalf("failed to decode XML: %v", err)
	}
	if err := pipe2rd.Close(); err != nil {
		t.Fatalf("failed to close pipe2rd: %v", err)
	}
	if err := eg.Wait(); err != nil {
		t.Fatalf("error during conversion: %v", err)
	}

	assertCoverage(t, v)
	p := v.Packages[0]
	assertPackage(t, p)
	c := p.Classes[0]
	assertClass(t, c)
	m := c.Methods[0]
	assertMethod(t, m)

	c = p.Classes[1]
	if c.Name != "Type1" {
		t.Errorf("expected class name 'Type1', got '%s'", c.Name)
	}
	if c.Filename != "testdata/func2.go" {
		t.Errorf("expected class filename 'testdata/func2.go', got '%s'", c.Filename)
	}
	if len(c.Methods) != 3 {
		t.Fatalf("expected 3 methods, got %d", len(c.Methods))
	}
}

func assertMethod(t *testing.T, m *Method) {
	t.Helper()

	if m.Name != "Func1" {
		t.Errorf("expected method name 'Func1', got '%s'", m.Name)
	}
	if len(m.Lines) != 4 {
		t.Fatalf("expected 4 lines in method, got %d", len(m.Lines))
	}

	if m.Lines[0].Number != 5 || m.Lines[0].Hits != 1 {
		t.Errorf("expected line 5 with 1 hit, got %d hits", m.Lines[0].Hits)
	}
	if m.Lines[1].Number != 6 || m.Lines[1].Hits != 0 {
		t.Errorf("expected line 6 with 0 hits, got %d hits", m.Lines[1].Hits)
	}
	if m.Lines[2].Number != 7 || m.Lines[2].Hits != 0 {
		t.Errorf("expected line 7 with 0 hits, got %d hits", m.Lines[2].Hits)
	}
	if m.Lines[3].Number != 8 || m.Lines[3].Hits != 0 {
		t.Errorf("expected line 8 with 0 hits, got %d hits", m.Lines[3].Hits)
	}
}

func assertClass(t *testing.T, c *Class) {
	t.Helper()

	if c.Name != "-" {
		t.Errorf("expected class name '-', got '%s'", c.Name)
	}
	if c.Filename != "testdata/func1.go" {
		t.Errorf("expected class filename 'testdata/func1.go', got '%s'", c.Filename)
	}
	if len(c.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(c.Methods))
	}
	if len(c.Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(c.Lines))
	}
	if c.Lines[0].Number != 5 || c.Lines[0].Hits != 1 {
		t.Errorf("expected line 5 with 1 hit, got %d hits", c.Lines[0].Hits)
	}
	if c.Lines[1].Number != 6 || c.Lines[1].Hits != 0 {
		t.Errorf("expected line 6 with 0 hits, got %d hits", c.Lines[1].Hits)
	}
	if c.Lines[2].Number != 7 || c.Lines[2].Hits != 0 {
		t.Errorf("expected line 7 with 0 hits, got %d hits", c.Lines[2].Hits)
	}
	if c.Lines[3].Number != 8 || c.Lines[3].Hits != 0 {
		t.Errorf("expected line 8 with 0 hits, got %d hits", c.Lines[3].Hits)
	}
}

func assertPackage(t *testing.T, p *Package) {
	t.Helper()

	if p.Name != "github.com/fasmat/gocover-cobertura/testdata" {
		t.Errorf("expected package name 'github.com/fasmat/gocover-cobertura/testdata', got '%s'", p.Name)
	}

	if len(p.Classes) != 2 {
		t.Fatalf("expected 2 classes, got %d", len(p.Classes))
	}
}

func assertCoverage(t *testing.T, v Coverage) {
	t.Helper()

	if v.XMLName.Local != "coverage" {
		t.Errorf("expected XML name 'coverage', got '%s'", v.XMLName.Local)
	}
	if len(v.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(v.Sources))
	}

	if len(v.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(v.Packages))
	}
}
