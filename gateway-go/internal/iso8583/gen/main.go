// Command gen reads contracts/iso8583/packager-spec.yaml and writes spec_gen.go.
// Run via `go generate ./...` from gateway-go/. Never edit spec_gen.go by hand.
package main

import (
	"fmt"
	"log"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

type rawField struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	Length    int    `yaml:"length"`
	Prefix    string `yaml:"prefix"`
	Sensitive string `yaml:"sensitive"`
}

type rawSpec struct {
	Fields map[int]rawField `yaml:"fields"`
}

func main() {
	data, err := os.ReadFile("../../../contracts/iso8583/packager-spec.yaml")
	if err != nil {
		log.Fatal(err)
	}
	var spec rawSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		log.Fatal(err)
	}
	numbers := make([]int, 0, len(spec.Fields))
	for n := range spec.Fields {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	out, err := os.Create("spec_gen.go")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	fmt.Fprintln(out, "// Code generated from contracts/iso8583/packager-spec.yaml by internal/iso8583/gen. DO NOT EDIT.")
	fmt.Fprintln(out, "package iso8583")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "// Fields is the MCN-87A field table (docs/03 §3).")
	fmt.Fprintln(out, "var Fields = map[int]FieldSpec{")
	for _, n := range numbers {
		f := spec.Fields[n]
		fmt.Fprintf(out, "\t%d: {Number: %d, Type: %q, Length: %d, Prefix: %q, Name: %q}, // %s\n",
			n, n, f.Type, f.Length, f.Prefix, f.Name, f.Name)
	}
	fmt.Fprintln(out, "}")
}
