package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dbvault/dbvault/internal/platform/kubernetes"
	"github.com/dbvault/dbvault/internal/platform/operator"
)

func main() {
	kind := flag.String("kind", "source", "source or repository")
	file := flag.String("file", "", "JSON resource file")
	flag.Parse()
	if *file == "" {
		fmt.Println("dbvault-operator scaffold: supply --kind source|repository --file resource.json")
		return
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	reconciler := operator.New()
	switch *kind {
	case "source":
		var resource kubernetes.DBVaultSource
		if err := json.Unmarshal(b, &resource); err != nil {
			fail(err)
		}
		status, err := reconciler.ReconcileSource(resource)
		if err != nil {
			fail(err)
		}
		printJSON(status)
	case "repository":
		var resource kubernetes.DBVaultRepository
		if err := json.Unmarshal(b, &resource); err != nil {
			fail(err)
		}
		status, err := reconciler.ReconcileRepository(resource)
		if err != nil {
			fail(err)
		}
		printJSON(status)
	default:
		fail(fmt.Errorf("unsupported kind %q", *kind))
	}
}
func fail(err error)  { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
func printJSON(v any) { b, _ := json.MarshalIndent(v, "", "  "); fmt.Println(string(b)) }
