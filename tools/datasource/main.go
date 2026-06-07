// Command datasource scaffolds the boilerplate for a new Omada provider data
// source. It lives in the separate `tools` module (it is a developer utility,
// not part of the provider's importable source) and emits Go source as text,
// so it does not import the provider packages.
//
// Usage (from the repo root):
//
//	make datasource NAME=<name>
//
// or directly, from within the tools module:
//
//	cd tools
//	go run ./datasource <name>
//	go run ./datasource -name <name>
//
// where <name> is the data source name in lower_snake_case, e.g. "device".
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/template"
)

// namePattern enforces lower_snake_case so the name maps cleanly onto both the
// Go package under internal/service and the "omada_<name>" Terraform type.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// templateData carries the values exposed to the code templates.
type templateData struct {
	// Name is the data source name in lower_snake_case (e.g. "device"). It
	// becomes the Terraform type suffix: "device" -> "omada_device".
	Name string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "datasource generator:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("datasource", flag.ContinueOnError)
	nameFlag := fs.String("name", "", `data source name in lower_snake_case, e.g. "device"`)
	if err := fs.Parse(args); err != nil {
		return err
	}

	name := *nameFlag
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return fmt.Errorf("a data source name is required, e.g. `go run ./datasource device`")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid name %q: use lower_snake_case starting with a letter", name)
	}

	data := templateData{Name: name}

	// TODO(next): render the full set of files into internal/service/<name>/
	// (data_source_*.go, model.go, flatten.go, *_test.go, docs/examples) and
	// register <name>.NewDataSourceList in internal/provider/provider.go.
	//
	// Note: this command runs from the tools module, so the working directory
	// is tools/. Output paths must be resolved relative to the repo root
	// (i.e. ../internal/service/<name>/), not the current directory.
	//
	// For now we render the registration snippet to stdout to confirm the
	// command and text/template wiring works end to end.
	if err := registrationTmpl.Execute(os.Stdout, data); err != nil {
		return fmt.Errorf("render registration snippet: %w", err)
	}

	fmt.Printf("planned package directory (relative to repo root): %s\n", filepath.Join("internal", "service", data.Name))
	return nil
}

// registrationTmpl is a placeholder template proving the generator can render
// Go source. Real file templates will be added in a follow-up.
var registrationTmpl = template.Must(template.New("registration").Parse(
	`// Add to (p *omadaProvider) DataSources in internal/provider/provider.go:
//     {{ .Name }}.NewDataSourceList,
// Terraform type: omada_{{ .Name }}
`))
