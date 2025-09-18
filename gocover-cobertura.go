// gocover-cobertura converts Go code coverage profiles to Cobertura XML format.
//
// It reads from standard input and writes to standard output.
// It can be used to generate code coverage reports compatible with tools that
// expect Cobertura format, such as SonarQube or Jenkins.
package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/tools/cover"
	"golang.org/x/tools/go/packages"
)

const coberturaDTDDecl = `<!DOCTYPE coverage SYSTEM "http://cobertura.sourceforge.net/xml/coverage-04.dtd">`

func printHelp() {
	fmt.Fprintf(os.Stderr, "gocover-cobertura converts Go code coverage profiles to Cobertura XML format.\n\n")

	fmt.Fprintf(os.Stderr, "By default it reads from stdin and writes to stdout.\n")
	fmt.Fprintf(os.Stderr, "It can be used to generate code coverage reports compatible with tools that\n")
	fmt.Fprintf(os.Stderr, "expect Cobertura format, such as SonarQube or Jenkins.\n\n")

	fmt.Fprintf(os.Stderr, "Note: this tool needs to be run in the root folder of the Go module (directory with\n")
	fmt.Fprintf(os.Stderr, "the go.mod file) that was used to produce the coverage profile.\n\n")

	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  go test -coverprofile=coverage.out ./...\n")
	if runtime.GOOS == "windows" {
		fmt.Fprintf(os.Stderr, "  type coverage.out | gocover-cobertura.exe > coverage.xml\n")
	} else {
		fmt.Fprintf(os.Stderr, "  cat coverage.out | gocover-cobertura > coverage.xml\n")
	}
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func main() {
	var ignore Ignore
	var byFiles bool
	var help bool
	inFile := os.Stdin
	outFile := os.Stdout

	inFileName := flag.String("f", "", "path to coverage file (default: stdin)")
	outFileName := flag.String("o", "", "path to output file (default: stdout)")
	flag.BoolVar(&help, "h", false, "show help")
	flag.BoolVar(&byFiles, "by-files", false, "code coverage by file, not class")
	flag.BoolVar(&ignore.GeneratedFiles, "ignore-gen-files", false, "ignore generated files")
	ignoreDirsRe := flag.String("ignore-dirs", "", "ignore dirs matching this regexp")
	ignoreFilesRe := flag.String("ignore-files", "", "ignore files matching this regexp")
	buildTags := flag.String("tags", "", "build tags to use when loading packages")
	flag.Parse()

	if help {
		printHelp()
		return
	}

	if *inFileName != "" {
		var err error
		inFile, err = os.Open(*inFileName)
		if err != nil {
			log.Fatalf("Failed to open input file %q: %s", *inFileName, err)
		}
		defer inFile.Close()
	}
	if *outFileName != "" {
		var err error
		err = os.MkdirAll(filepath.Dir(*outFileName), 0o755)
		if err != nil && !errors.Is(err, fs.ErrExist) {
			log.Fatalf("Failed to create output directory for %q: %s", *outFileName, err)
		}
		outFile, err = os.Create(*outFileName)
		if err != nil {
			log.Fatalf("Failed to create output file %q: %s", *outFileName, err)
		}
		defer outFile.Close()
	}

	var err error
	if *ignoreDirsRe != "" {
		ignore.Dirs, err = regexp.Compile(*ignoreDirsRe)
		if err != nil {
			log.Fatalf("Bad -ignore-dirs regexp: %s", err)
		}
	}

	if *ignoreFilesRe != "" {
		ignore.Files, err = regexp.Compile(*ignoreFilesRe)
		if err != nil {
			log.Fatalf("Bad -ignore-files regexp: %s", err)
		}
	}

	if *buildTags != "" {
		log.Printf("Using build tags: %s", *buildTags)
	}

	if err := convert(inFile, outFile, &ignore, byFiles, *buildTags); err != nil {
		log.Fatalf("code coverage conversion failed: %s", err)
	}
}

func convert(in io.Reader, out io.Writer, ignore *Ignore, byFiles bool, buildTags string) error {
	ignoreRd := NewIgnoreReader(ignore, in)
	profiles, err := cover.ParseProfilesFromReader(ignoreRd)
	if err != nil {
		return fmt.Errorf("parse profiles: %w", err)
	}

	pkgs, err := getPackages(profiles, buildTags)
	if err != nil {
		return fmt.Errorf("get packages: %w", err)
	}

	sources := make([]*Source, 0)
	pkgMap := make(map[string]*packages.Package)
	for _, pkg := range pkgs {
		sources = appendIfUnique(sources, pkg.Module.Dir)
		pkgMap[pkg.ID] = pkg
	}

	coverage := Coverage{
		Sources:   sources,
		Packages:  nil,
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}
	if err := coverage.parseProfiles(profiles, pkgMap, ignore, byFiles); err != nil {
		return fmt.Errorf("parse coverage profiles: %w", err)
	}

	if _, err := fmt.Fprint(out, xml.Header); err != nil {
		return fmt.Errorf("write XML header: %w", err)
	}
	if _, err := fmt.Fprintln(out, coberturaDTDDecl); err != nil {
		return fmt.Errorf("write DTD declaration: %w", err)
	}

	encoder := xml.NewEncoder(out)
	encoder.Indent("", "  ")
	if err := encoder.Encode(coverage); err != nil {
		return fmt.Errorf("encode coverage: %w", err)
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return fmt.Errorf("write XML footer: %w", err)
	}
	return nil
}

func getPackages(profiles []*cover.Profile, buildTags string) ([]*packages.Package, error) {
	if len(profiles) == 0 {
		return []*packages.Package{}, nil
	}

	var pkgNames []string
	for _, profile := range profiles {
		pkgNames = append(pkgNames, getPackageName(profile.FileName))
	}
	buildTags = "-tags=" + buildTags
	cfg := &packages.Config{
		Mode:       packages.NeedFiles | packages.NeedModule,
		BuildFlags: []string{buildTags},
	}
	return packages.Load(cfg, pkgNames...)
}

func appendIfUnique(sources []*Source, dir string) []*Source {
	for _, source := range sources {
		if source.Path == dir {
			return sources
		}
	}
	return append(sources, &Source{dir})
}

func getPackageName(filename string) string {
	pkgName, _ := filepath.Split(filename)
	// TODO(boumenot): Windows vs. Linux
	return strings.TrimRight(strings.TrimRight(pkgName, "\\"), "/")
}

func findAbsFilePath(pkg *packages.Package, profileName string) string {
	filename := filepath.Base(profileName)
	for _, fullPath := range pkg.GoFiles {
		if filepath.Base(fullPath) == filename {
			return fullPath
		}
	}
	return ""
}

func (cov *Coverage) parseProfiles(
	profiles []*cover.Profile,
	pkgMap map[string]*packages.Package,
	ignore *Ignore,
	byFiles bool,
) error {
	cov.Packages = []*Package{}
	for _, profile := range profiles {
		pkgName := getPackageName(profile.FileName)
		pkgPkg := pkgMap[pkgName]
		if err := cov.parseProfile(profile, pkgPkg, ignore, byFiles); err != nil {
			return err
		}
	}
	cov.LinesValid = cov.NumLines()
	cov.LinesCovered = cov.NumLinesWithHits()
	cov.LineRate = cov.HitRate()
	return nil
}

func (cov *Coverage) parseProfile(
	profile *cover.Profile,
	pkgPkg *packages.Package,
	ignore *Ignore,
	byFiles bool,
) error {
	if pkgPkg == nil || pkgPkg.Module == nil {
		return errors.New("package required when using go modules")
	}
	fileName := profile.FileName[len(pkgPkg.Module.Path)+1:]
	absFilePath := findAbsFilePath(pkgPkg, profile.FileName)
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, absFilePath, nil, 0)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(absFilePath)
	if err != nil {
		return err
	}

	if ignore.Match(fileName, data) {
		return nil
	}

	pkgPath, _ := filepath.Split(fileName)
	pkgPath = strings.TrimRight(strings.TrimRight(pkgPath, "/"), "\\")
	pkgPath = filepath.Join(pkgPkg.Module.Path, pkgPath)
	// TODO(boumenot): package paths are not file paths, there is a consistent separator
	pkgPath = strings.ReplaceAll(pkgPath, "\\", "/")

	var pkg *Package
	for _, p := range cov.Packages {
		if p.Name == pkgPath {
			pkg = p
		}
	}
	if pkg == nil {
		pkg = &Package{Name: pkgPkg.ID, Classes: []*Class{}}
		cov.Packages = append(cov.Packages, pkg)
	}
	visitor := &fileVisitor{
		fset:     fset,
		fileName: fileName,
		fileData: data,
		byFiles:  byFiles,
		classes:  make(map[string]*Class),
		pkg:      pkg,
		profile:  profile,
	}
	ast.Walk(visitor, parsed)
	pkg.LineRate = pkg.HitRate()
	return nil
}

type fileVisitor struct {
	fset     *token.FileSet
	fileName string
	fileData []byte
	pkg      *Package
	byFiles  bool
	classes  map[string]*Class
	profile  *cover.Profile
}

func (v *fileVisitor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.FuncDecl:
		class := v.class(n)
		method := v.method(n)
		method.LineRate = method.Lines.HitRate()
		class.Methods = append(class.Methods, method)
		class.Lines = append(class.Lines, method.Lines...)

		class.LineRate = class.Lines.HitRate()
	}
	return v
}

func (v *fileVisitor) method(n *ast.FuncDecl) *Method {
	method := &Method{Name: n.Name.Name}
	method.Lines = []*Line{}

	start := v.fset.Position(n.Pos())
	end := v.fset.Position(n.End())
	startLine := start.Line
	startCol := start.Column
	endLine := end.Line
	endCol := end.Column
	// The blocks are sorted, so we can stop counting as soon as we reach the end of the relevant block.
	for _, b := range v.profile.Blocks {
		if b.StartLine > endLine || (b.StartLine == endLine && b.StartCol >= endCol) {
			// Past the end of the function.
			break
		}
		if b.EndLine < startLine || (b.EndLine == startLine && b.EndCol <= startCol) {
			// Before the beginning of the function
			continue
		}
		for i := b.StartLine; i <= b.EndLine; i++ {
			method.Lines.AddOrUpdateLine(i, int64(b.Count))
		}
	}
	return method
}

func (v *fileVisitor) class(n *ast.FuncDecl) *Class {
	var className string
	if v.byFiles {
		// NOTE(boumenot): ReportGenerator creates links that collide if names are not distinct.
		// This could be an issue in how I am generating the report, but I have not been able
		// to figure it out.  The work around is to generate a fully qualified name based on
		// the file path.
		//
		// src/lib/util/foo.go -> src.lib.util.foo.go
		className = strings.ReplaceAll(v.fileName, "/", ".")
		className = strings.ReplaceAll(className, "\\", ".")
	} else {
		className = v.recvName(n)
	}
	class := v.classes[className]
	if class == nil {
		class = &Class{Name: className, Filename: v.fileName, Methods: []*Method{}, Lines: []*Line{}}
		v.classes[className] = class
		v.pkg.Classes = append(v.pkg.Classes, class)
	}
	return class
}

func (v *fileVisitor) recvName(n *ast.FuncDecl) string {
	if n.Recv == nil {
		return "-"
	}
	recv := n.Recv.List[0].Type
	start := v.fset.Position(recv.Pos())
	end := v.fset.Position(recv.End())
	name := string(v.fileData[start.Offset:end.Offset])
	return strings.TrimSpace(strings.TrimLeft(name, "*"))
}
