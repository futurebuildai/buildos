// bench-gate parses Go benchmark output and enforces latency thresholds.
// Used by `make audit` as a CI hard gate for physics engine performance.
//
// Usage:
//
//	go test -bench=BenchmarkCPM -benchtime=10x ./internal/physics/... -run='^$' |
//	    go run ./tools/bench-gate --cpm80=200ms --cpm200=500ms
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	cpm80 := flag.Duration("cpm80", 200*time.Millisecond, "Max latency for BenchmarkCPM80Tasks")
	cpm200 := flag.Duration("cpm200", 500*time.Millisecond, "Max latency for BenchmarkCPM200Tasks")
	flag.Parse()

	thresholds := map[string]time.Duration{
		"BenchmarkCPM80Tasks":  *cpm80,
		"BenchmarkCPM200Tasks": *cpm200,
	}

	// Parse benchmark output from stdin
	// Format: BenchmarkCPM80Tasks-14    10    189882 ns/op
	benchRe := regexp.MustCompile(`^(Benchmark\w+)-\d+\s+\d+\s+(\d+(?:\.\d+)?)\s+ns/op`)

	results := make(map[string]time.Duration)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line) // Pass through to stdout

		matches := benchRe.FindStringSubmatch(line)
		if len(matches) >= 3 {
			name := matches[1]
			nsStr := matches[2]

			// Parse ns/op (may be float like 189882.5)
			ns, err := strconv.ParseFloat(nsStr, 64)
			if err != nil {
				continue
			}
			results[name] = time.Duration(ns)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}

	// Check thresholds
	failed := false
	for name, threshold := range thresholds {
		actual, ok := results[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "GATE FAIL: %s not found in benchmark output\n", name)
			failed = true
			continue
		}

		if actual > threshold {
			fmt.Fprintf(os.Stderr, "GATE FAIL: %s took %v (threshold: %v)\n",
				name, actual, threshold)
			failed = true
		} else {
			fmt.Fprintf(os.Stderr, "GATE PASS: %s took %v (threshold: %v) [%.1f%% headroom]\n",
				name, actual, threshold,
				float64(threshold-actual)/float64(threshold)*100)
		}
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "GATE FAIL: No benchmark results found in output")
		failed = true
	}

	// Print summary for missing thresholds that had results
	for name, actual := range results {
		if _, hasThreshold := thresholds[name]; !hasThreshold {
			fmt.Fprintf(os.Stderr, "INFO: %s = %v (no threshold configured)\n", name, actual)
		}
	}

	if failed {
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, strings.Repeat("-", 50))
	fmt.Fprintln(os.Stderr, "Physics engine benchmark gates: ALL PASSED")
}
