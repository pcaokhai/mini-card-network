// Command gen reads contracts/iso8583/packager-spec.yaml and writes spec_gen.go.
// Run via `go generate ./...` from gateway-go/. Never edit spec_gen.go by hand.
package main

import (
	"bytes"
	"fmt"
	"go/format"
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

	var buf bytes.Buffer
	fmt.Fprintln(&buf, "// Code generated from contracts/iso8583/packager-spec.yaml by internal/iso8583/gen. DO NOT EDIT.")
	fmt.Fprintln(&buf, "package iso8583")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "// Fields is the MCN-87A field table (docs/03 §3).")
	fmt.Fprintln(&buf, "var Fields = map[int]FieldSpec{")
	for _, n := range numbers {
		f := spec.Fields[n]
		fmt.Fprintf(&buf, "\t%d: {Number: %d, Type: %q, Length: %d, Prefix: %q, Name: %q}, // %s\n",
			n, n, f.Type, f.Length, f.Prefix, f.Name, f.Name)
	}
	fmt.Fprintln(&buf, "}")

	// Format before writing so `go generate` always produces gofmt-clean, committable output.
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("spec_gen.go", formatted, 0o600); err != nil {
		log.Fatal(err)
	}
}
