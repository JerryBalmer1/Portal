// Command graphnode validates graph envelopes and ontology files against the
// GraphNode schemas, and prints the schemas themselves.
//
//	graphnode validate <graph.json> [--ontology <ontology.yaml>]  exit 1 if any error
//	graphnode validate-ontology <ontology.yaml>                   exit 1 if any error
//	graphnode prefixes                                            print the registered id prefixes as JSON
//	graphnode check-id <prefix> <id>                              exit 1 if id does not have the prefix's shape
//	graphnode schema [name]                                       print a schema, or list names
//	graphnode version
//
// Problems print as JSON with a severity; warnings alone exit 0.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"go.yaml.in/yaml/v3"
)

const version = "0.2.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "validate":
		need(3)
		file, ontology := "", ""
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "--ontology" && i+1 < len(os.Args) {
				ontology = os.Args[i+1]
				i++
			} else if file == "" {
				file = os.Args[i]
			} else {
				usage()
				os.Exit(2)
			}
		}
		data, err := os.ReadFile(file)
		die(err)
		if ontology == "" {
			report(graph.ValidateBytes(data))
			return
		}
		report(graph.ValidateWithOntology(data, readOntology(ontology)))
	case "validate-ontology":
		need(3)
		report(graph.ValidateOntology(readOntology(os.Args[2])))
	case "prefixes":
		b, _ := json.MarshalIndent(graph.IDPrefixes(), "", "  ")
		os.Stdout.Write(b)
		fmt.Println()
	case "check-id":
		need(4)
		if err := graph.ValidID(os.Args[2], os.Args[3]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "schema":
		if len(os.Args) == 2 {
			for _, n := range graph.SchemaNames() {
				fmt.Println(n)
			}
			return
		}
		b, err := graph.SchemaBytes(os.Args[2])
		die(err)
		os.Stdout.Write(b)
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func report(ps []graph.Problem) {
	b, _ := graph.MarshalProblems(ps)
	os.Stdout.Write(b)
	fmt.Println()
	if graph.HasErrors(ps) {
		os.Exit(1)
	}
}

// readOntology loads an ontology.yaml (or .json) as plain JSON-shaped data.
func readOntology(path string) any {
	data, err := os.ReadFile(path)
	die(err)
	var doc any
	die(yaml.Unmarshal(data, &doc))
	return normaliseYAML(doc)
}

// normaliseYAML turns map[any]any (older yaml decoders) into map[string]any so
// the schema validator sees plain JSON-shaped data.
func normaliseYAML(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normaliseYAML(val)
		}
		return t
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normaliseYAML(val)
		}
		return m
	case []any:
		for i, val := range t {
			t[i] = normaliseYAML(val)
		}
		return t
	default:
		return v
	}
}

func need(n int) {
	if len(os.Args) < n {
		usage()
		os.Exit(2)
	}
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "graphnode:", err)
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: graphnode validate <graph.json> [--ontology <ontology.yaml>] | validate-ontology <ontology.yaml> | prefixes | check-id <prefix> <id> | schema [name] | version")
}
