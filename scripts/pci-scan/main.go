package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Walk scans every file under paths (skipping directories/files that match an allowlist entry
// by path suffix) line by line with all three detectors.
func Walk(paths []string, allowlist []string) ([]Finding, error) {
	var findings []Finding
	for _, root := range paths {
		info, err := os.Stat(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			fs, err := scanFile(root, allowlist)
			if err != nil {
				return nil, err
			}
			findings = append(findings, fs...)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			fs, err := scanFile(path, allowlist)
			if err != nil {
				return err
			}
			findings = append(findings, fs...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return findings, nil
}

func scanFile(path string, allowlist []string) ([]Finding, error) {
	normalized := filepath.ToSlash(path)
	for _, a := range allowlist {
		if strings.HasSuffix(normalized, a) {
			return nil, nil
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []Finding
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		for _, fn := range []func(string) []Finding{ScanPAN, ScanPinBlock, ScanTrack2} {
			for _, finding := range fn(line) {
				finding.File = path
				finding.Line = lineNo
				findings = append(findings, finding)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return findings, nil
}

func main() {
	var paths []string
	for _, p := range os.Args[1:] {
		if p != "" {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		paths = []string{"infra/logs", "contracts/fixtures"}
	}
	if dump := os.Getenv("PCI_SCAN_DB_DUMP"); dump != "" {
		paths = append(paths, dump)
	}

	findings, err := Walk(paths, []string{"contracts/fixtures/cards.json"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "pci-scan error:", err)
		os.Exit(1)
	}

	for _, f := range findings {
		fmt.Printf("%s:%d [%s] %s\n", f.File, f.Line, f.Detector, f.Masked)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}
