# gocover-cobertura

[![Build Status](https://img.shields.io/github/actions/workflow/status/fasmat/gocover-cobertura/ci.yml)](https://github.com/fasmat/gocover-cobertura/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/fasmat/gocover-cobertura/graph/badge.svg?token=2WFR1O5B42)](https://codecov.io/gh/fasmat/gocover-cobertura)
[![Go Report Card](https://goreportcard.com/badge/github.com/fasmat/gocover-cobertura)](https://goreportcard.com/report/github.com/fasmat/gocover-cobertura)
[![Go Reference](https://pkg.go.dev/badge/github.com/fasmat/gocover-cobertura?status.svg)](https://pkg.go.dev/github.com/fasmat/gocover-cobertura?tab=doc)
[![License](https://img.shields.io/github/license/fasmat/gocover-cobertura)](./LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/fasmat/gocover-cobertura)](https://github.com/fasmat/gocover-cobertura/releases/latest)

This is a **fork** of <https://github.com/boumenot/gocover-cobertura>.

At the time of this writing the repository appears to be on *pause* with several outstanding PRs, and open issues.
The main motivator for creating this fork was to add the ability to pass build tags to the converter that were used
when recording the coverage.

Additionally the following changes were made:

- allow passing build tags to converter (see `-tags` flag)
- update the code base to a more recent version of go
- instead of forking `golang.org/x/tools/cover` to parse the coverage profile, use the package directly
- reduce the number of dependencies (3 vs 7)
- stricter linting of the codebase with `golangci-lint`
- increase test coverage
- light improvements to CLI (add help with `-h`, allow for inputs/outputs other than stdin/stdout)

This is a simple helper tool for generating XML output in [Cobertura](http://cobertura.sourceforge.net/) format
for CIs like [Jenkins](https://wiki.jenkins-ci.org/display/JENKINS/Cobertura+Plugin) and others
from [go tool cover](https://github.com/golang/go/tree/master/src/cmd/cover/) output.

## Installation

Just type the following to install the program and its dependencies:

```shell
go install github.com/fasmat/gocover-cobertura@latest
```

## Usage

`gocover-cobertura` reads from the standard input:

```bash
go test -coverprofile=coverage.txt -covermode count github.com/gorilla/mux
gocover-cobertura < coverage.txt > coverage.xml
```

Note that you should run this from the directory which holds your `go.mod` file, so the tool can match the profile to
the source files.

Some flags can be passed (each flag should only be used once):

- `-h`

  show help

- `-f FILENAME`

  The relative or absolute path to coverage file that should be converted (default: stdin)

- `-o FILENAME`

  The relative or absolute path to output file for the cobertura report (default: stdout)

- `-by-files`

  Code coverage is organized by class by default. This flag organizes code
  coverage by the name of the file, which the same behavior as `go tool cover`.

- `-ignore-dirs PATTERN`

  ignore directories matching `PATTERN` regular expression. Full directory names are matched, examples of use:

  ```shell
  # A specific directory
  -ignore-dirs '^github\.com/fasmat/gocover-cobertura/testdata$'
  # All directories called "autogen" and any of their sub-directories
  -ignore-dirs '/autogen$'
  ```

- `-ignore-files PATTERN`

  ignore files matching `PATTERN` regular expression. Full file names are matched, examples of use:

  ```shell
  # A specific file
  -ignore-files '^github\.com/fasmat/gocover-cobertura/profile\.go$'
  # All files ending with _gen.go
  -ignore-files '_gen\.go$'
  # All files in a directory autogen (or any of its subdirs)
  -ignore-files '/autogen/'
  ```

- `-ignore-gen-files`

  ignore generated files. Typically files containing a comment indicating that the file has been automatically
  generated. See `genCodeRe` regexp in [ignore.go](ignore.go).

- `-tags`

  comma-separated list of build tags to consider when looking for source files. This should match the build tags
  used when running `go test -coverprofile=...`.
