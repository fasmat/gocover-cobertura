# TODOs

- Update README.md with new features
- allow passing the coverage profile from a file (instead of os.Stdin but keep option to read from stdin)
- allow specifying output file for the XML report (instead of os.Stdout but default to stdout if not specified)
- check if `ParseProfilesFromReader` can be used without forking it
  - should just be called from [golang.org/x/tools](https://pkg.go.dev/golang.org/x/tools/cover)
  - filtering can probably be done on the result of the call to this function
- Add tests for new functionality
- Release proper version
