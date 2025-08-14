# gocover-cobertura

This is a **fork** of https://github.com/boumenot/gocover-cobertura.

At the time of this writing the repository appears to be on *pause* with
several outstanding PRs, and forks with interesting contributions.  This
repo consolidates those outstanding forks, and combines them into one repo.

This is a simple helper tool for generating XML output in [Cobertura](http://cobertura.sourceforge.net/) format
for CIs like [Jenkins](https://wiki.jenkins-ci.org/display/JENKINS/Cobertura+Plugin) and others
from [go tool cover](https://code.google.com/p/go.tools/) output.

## Installation

Just type the following to install the program and its dependencies:

```shell
go get github.com/fasmat/gocover-cobertura
```

## Usage

`gocover-cobertura` reads from the standard input:

    $ go test -coverprofile=coverage.txt -covermode count github.com/gorilla/mux
    $ gocover-cobertura < coverage.txt > coverage.xml
    
Note that you should run this from the directory which holds your `go.mod` file.

Some flags can be passed (each flag should only be used once):

- `-by-files`

  Code coverage is organized by class by default.  This flag organizes code
  coverage by the name of the file, which the same behavior as `go tool cover`.

- `-ignore-dirs PATTERN`

  ignore directories matching `PATTERN` regular expression. Full
  directory names are matched, as
  `github.com/fasmat/gocover-cobertura` (and so `github.com/fasmat`
  and `github.com`), examples of use:
  ```
  # A specific directory
  -ignore-dirs '^github\.com/boumenot/gocover-cobertura/testdata$'
  # All directories autogen and any of their subdirs
  -ignore-dirs '/autogen$'
  ```

- `-ignore-files PATTERN`

  ignore files matching `PATTERN` regular expression. Full file names
  are matched, as `github.com/fasmat/gocover-cobertura/profile.go`,
  examples of use:
  ```
  # A specific file
  -ignore-files '^github\.com/boumenot/gocover-cobertura/profile\.go$'
  # All files ending with _gen.go
  -ignore-files '_gen\.go$'
  # All files in a directory autogen (or any of its subdirs)
  -ignore-files '/autogen/'
  ```

- `-ignore-gen-files`

  ignore generated files. Typically files containing a comment
  indicating that the file has been automatically generated. See
  `genCodeRe` regexp in [ignore.go](ignore.go).
