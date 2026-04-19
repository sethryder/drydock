package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const usage = `drydock — inspect helm_release value changes in a Terraform plan before you set sail

Usage:
  drydock <plan.tfplan>           # binary plan, shells out to terraform show -json
  drydock <plan.json>             # already-rendered JSON plan
  drydock -                       # read JSON plan from stdin
  drydock plan [-- tf-args...]    # run ` + "`terraform plan`" + ` and diff in one step
  terraform show -json p.tfplan | drydock -

Flags (apply to all forms):
`

func main() {
	// Split subcommand out before flag parsing so `plan` can pass through its own args.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "plan" {
		if err := runPlanSubcommand(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "drydock:%v\n", err)
			os.Exit(1)
		}
		return
	}

	fs := flag.NewFlagSet("drydock", flag.ExitOnError)
	noColor := fs.Bool("no-color", false, "disable ANSI color output")
	onlyRelease := fs.String("release", "", "only show diff for the helm_release with this address (e.g. helm_release.airflow)")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	raw, err := loadPlanJSON(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "drydock:%v\n", err)
		os.Exit(1)
	}

	if err := renderPlanJSON(raw, *onlyRelease, *noColor); err != nil {
		fmt.Fprintf(os.Stderr, "drydock:%v\n", err)
		os.Exit(1)
	}
}

// runPlanSubcommand shells out to `terraform plan -out=<tmp>`, then
// `terraform show -json <tmp>`, and pipes the result into the diff renderer.
// Unknown flags pass through to `terraform plan` so callers can use -var,
// -target, -var-file, -chdir, etc. drydock-specific flags (--no-color,
// --release, --chdir) are extracted first.
func runPlanSubcommand(args []string) error {
	var (
		noColor     bool
		onlyRelease string
		chdir       string
		tfArgs      []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			tfArgs = append(tfArgs, args[i+1:]...)
			i = len(args)
		case a == "--no-color":
			noColor = true
		case a == "--release":
			if i+1 >= len(args) {
				return fmt.Errorf("--release requires a value")
			}
			onlyRelease = args[i+1]
			i++
		case len(a) > len("--release=") && a[:len("--release=")] == "--release=":
			onlyRelease = a[len("--release="):]
		case a == "--chdir":
			if i+1 >= len(args) {
				return fmt.Errorf("--chdir requires a value")
			}
			chdir = args[i+1]
			i++
		case len(a) > len("--chdir=") && a[:len("--chdir=")] == "--chdir=":
			chdir = a[len("--chdir="):]
		default:
			tfArgs = append(tfArgs, a)
		}
	}

	tmpDir, err := os.MkdirTemp("", "drydock-")
	if err != nil {
		return fmt.Errorf("creating tempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	planFile := filepath.Join(tmpDir, "plan.tfplan")

	planCmd := []string{}
	if chdir != "" {
		planCmd = append(planCmd, "-chdir="+chdir)
	}
	planCmd = append(planCmd, "plan", "-out="+planFile)
	planCmd = append(planCmd, tfArgs...)

	tf := exec.Command("terraform", planCmd...)
	tf.Stdout = os.Stderr // keep stdout clean for piping; plan progress goes to stderr
	tf.Stderr = os.Stderr
	tf.Stdin = os.Stdin
	if err := tf.Run(); err != nil {
		return fmt.Errorf("terraform plan: %w", err)
	}

	showArgs := []string{}
	if chdir != "" {
		showArgs = append(showArgs, "-chdir="+chdir)
	}
	showArgs = append(showArgs, "show", "-json", planFile)
	show := exec.Command("terraform", showArgs...)
	var jsonOut []byte
	jsonOut, err = show.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("terraform show: %s", ee.Stderr)
		}
		return fmt.Errorf("terraform show: %w", err)
	}

	fmt.Fprintln(os.Stderr) // visual separator between tf output and diff
	return renderPlanJSON(jsonOut, onlyRelease, noColor)
}

func renderPlanJSON(raw []byte, onlyRelease string, noColor bool) error {
	var plan Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return fmt.Errorf("parsing plan JSON: %w", err)
	}

	releases := HelmReleaseChanges(&plan)
	if onlyRelease != "" {
		filtered := releases[:0]
		for _, r := range releases {
			if r.Address == onlyRelease {
				filtered = append(filtered, r)
			}
		}
		releases = filtered
	}

	if len(releases) == 0 {
		fmt.Println("No helm_release changes found.")
		return nil
	}

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	opts := RenderOptions{Color: !noColor && isTerminal(os.Stdout)}
	for _, rc := range releases {
		RenderRelease(w, rc, opts)
	}
	return nil
}

func loadPlanJSON(arg string) ([]byte, error) {
	if arg == "-" {
		return io.ReadAll(os.Stdin)
	}
	f, err := os.Open(arg)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var first [1]byte
	n, _ := f.Read(first[:])
	if n == 0 {
		return nil, fmt.Errorf("empty input file")
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	if first[0] == '{' {
		return io.ReadAll(f)
	}

	cmd := exec.Command("terraform", "show", "-json", arg)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("terraform show failed: %s", ee.Stderr)
		}
		return nil, fmt.Errorf("running terraform show: %w", err)
	}
	return out, nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
