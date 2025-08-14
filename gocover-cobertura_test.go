package main

import (
	"encoding/xml"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"
)

func Test_Main(t *testing.T) {
	t.Parallel()

	fname := filepath.Join(t.TempDir(), "stdout")
	temp, err := os.Create(fname)
	require.NoError(t, err)
	stdout := os.Stdout
	defer func() {
		os.Stdout = stdout
	}()
	os.Stdout = temp
	main()
	os.Stdout = stdout
	require.NoError(t, temp.Close())
	outputBytes, err := os.ReadFile(fname)
	require.NoError(t, err)

	outputString := string(outputBytes)
	require.Contains(t, outputString, xml.Header)
	require.Contains(t, outputString, coberturaDTDDecl)
}

func TestConvertParseProfilesError(t *testing.T) {
	t.Parallel()

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	defer pipe2wr.Close()

	err := convert(strings.NewReader("invalid data"), pipe2wr, &Ignore{}, "")
	require.ErrorContains(t, err, "bad mode line: invalid data")
}

func TestConvertOutputError(t *testing.T) {
	t.Parallel()

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	require.NoError(t, pipe2wr.Close())

	err := convert(strings.NewReader("mode: set"), pipe2wr, &Ignore{}, "")
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestConvertEmpty(t *testing.T) {
	t.Parallel()

	data := `mode: set`

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	defer pipe2wr.Close()

	var eg errgroup.Group
	eg.Go(func() error {
		return convert(strings.NewReader(data), pipe2wr, &Ignore{}, "")
	})
	defer func() {
		require.NoError(t, eg.Wait())
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err := dec.Decode(&v)
	require.NoError(t, err)

	require.Equal(t, "coverage", v.XMLName.Local)
	require.Nil(t, v.Sources)
	require.Nil(t, v.Packages)
	require.NoError(t, pipe2rd.Close())
}

func TestParseProfileNilPackages(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, nil, &Ignore{})
	require.Error(t, err)
	require.Contains(t, `package required when using go modules`, err.Error())
}

func TestParseProfileEmptyPackages(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, &packages.Package{}, &Ignore{})
	require.Error(t, err)
	require.Contains(t, `package required when using go modules`, err.Error())
}

func TestParseProfileDoesNotExist(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}

	pkg := packages.Package{
		Name:   "does-not-exist",
		Module: &packages.Module{},
	}

	err := v.parseProfile(&profile, &pkg, &Ignore{})
	pathErr := &fs.PathError{}
	require.ErrorAs(t, err, &pathErr)
}

func TestParseProfileNotReadable(t *testing.T) {
	t.Parallel()

	v := Coverage{}
	profile := Profile{FileName: os.DevNull}
	err := v.parseProfile(&profile, nil, &Ignore{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "package required when using go modules")
}

func TestParseProfilePermissionDenied(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("chmod is not supported by Windows")
	}

	tempFile, err := os.CreateTemp(t.TempDir(), "not-readable")
	require.NoError(t, err)

	err = tempFile.Chmod(0o000)
	require.NoError(t, err)
	v := Coverage{}
	profile := Profile{FileName: tempFile.Name()}
	pkg := packages.Package{
		GoFiles: []string{
			tempFile.Name(),
		},
		Module: &packages.Module{
			Path: filepath.Dir(tempFile.Name()),
		},
	}
	err = v.parseProfile(&profile, &pkg, &Ignore{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}

func TestConvertSetMode(t *testing.T) {
	t.Parallel()

	src, err := os.Open("testdata/testdata_set.txt")
	require.NoError(t, err)
	defer src.Close()

	pipe2rd, pipe2wr := io.Pipe()
	defer pipe2rd.Close()
	defer pipe2wr.Close()

	var eg errgroup.Group
	eg.Go(func() error {
		return convert(src, pipe2wr, &Ignore{
			GeneratedFiles: true,
			Files:          regexp.MustCompile(`[\\/]func[45]\.go$`),
		}, "testdata")
	})
	defer func() {
		require.NoError(t, eg.Wait())
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	require.NoError(t, err)

	require.NoError(t, pipe2rd.Close())

	require.Equal(t, "coverage", v.XMLName.Local)
	require.Len(t, v.Sources, 1)
	require.Len(t, v.Packages, 1)

	p := v.Packages[0]
	require.Equal(t, "github.com/fasmat/gocover-cobertura/testdata", strings.TrimRight(p.Name, "/"))
	require.NotNil(t, p.Classes)
	require.Len(t, p.Classes, 2)

	c := p.Classes[0]
	require.Equal(t, "-", c.Name)
	require.Equal(t, "testdata/func1.go", c.Filename)
	require.NotNil(t, c.Methods)
	require.Len(t, c.Methods, 1)
	require.NotNil(t, c.Lines)
	require.Len(t, c.Lines, 4)

	m := c.Methods[0]
	require.Equal(t, "Func1", m.Name)
	require.NotNil(t, c.Lines)
	require.Len(t, c.Lines, 4)

	require.Equal(t, 5, m.Lines[0].Number)
	require.Equal(t, int64(1), m.Lines[0].Hits)
	require.Equal(t, 6, m.Lines[1].Number)
	require.Equal(t, int64(0), m.Lines[1].Hits)
	require.Equal(t, 7, m.Lines[2].Number)
	require.Equal(t, int64(0), m.Lines[2].Hits)
	require.Equal(t, 8, m.Lines[3].Number)
	require.Equal(t, int64(0), m.Lines[3].Hits)

	require.Equal(t, 5, c.Lines[0].Number)
	require.Equal(t, int64(1), c.Lines[0].Hits)
	require.Equal(t, 6, c.Lines[1].Number)
	require.Equal(t, int64(0), c.Lines[1].Hits)
	require.Equal(t, 7, c.Lines[2].Number)
	require.Equal(t, int64(0), c.Lines[2].Hits)
	require.Equal(t, 8, c.Lines[3].Number)
	require.Equal(t, int64(0), c.Lines[3].Hits)

	c = p.Classes[1]
	require.Equal(t, "Type1", c.Name)
	require.Equal(t, "testdata/func2.go", c.Filename)
	require.NotNil(t, c.Methods)
	require.Len(t, c.Methods, 3)
}
