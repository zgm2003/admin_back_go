package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"admin_back_go/internal/admincontract"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: admin-contract <generate|check> -out <directory> -commit <git-sha>")
		return 2
	}
	command := args[0]
	if command != "generate" && command != "check" {
		fmt.Fprintf(stderr, "unknown command %q\n", command)
		return 2
	}

	flags := flag.NewFlagSet("admin-contract "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("out", "contracts/admin/v1", "output directory")
	commit := flags.String("commit", "", "explicit full backend Git commit")
	checkMode := flags.Bool("check", false, "byte-compare instead of writing")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n", flags.Args())
		return 2
	}

	bundle, err := admincontract.Build(admincontract.BuildOptions{BackendCommit: *commit})
	if err != nil {
		fmt.Fprintf(stderr, "build Admin contract bundle: %v\n", err)
		return 1
	}
	if command == "check" || *checkMode {
		if err := admincontract.Check(*output, bundle); err != nil {
			fmt.Fprintf(stderr, "Admin contract check failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "Admin contract bundle is current.")
		return 0
	}
	if err := admincontract.WriteAtomic(*output, bundle); err != nil {
		fmt.Fprintf(stderr, "write Admin contract bundle: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Generated Admin contract bundle at %s.\n", *output)
	return 0
}
