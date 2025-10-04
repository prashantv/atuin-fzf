package main

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/prashantv/atuin-fzf/tcolor"
)

const _delim = "\t:::\t"

// TODOs:
// Consider replacing the emoji X with a red indicator of exit status.
// Add fzf bind to go to a dir AND exec
// Bind to Ctrl-R

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--preview" {
		if len(os.Args) > 2 {
			if err := fzfPreview(os.Args[2]); err != nil {
				log.Fatal(err)
			}
			return
		}
		return
	}

	var initialQuery string
	if len(os.Args) > 1 {
		initialQuery = os.Args[1]
	}

	if err := run(initialQuery); err != nil {
		log.Fatal(err)
	}
}

func run(query string) error {
	results, err := runAtuin(atuinParams{
		Limit: 1000,
	})
	if err != nil {
		return err
	}

	fzfInput, err := atuinToFzf(results)
	if err != nil {
		return err
	}

	if err := fzf(fzfInput, query); err != nil {
		return err
	}

	return nil
}

func atuinToFzf(results iter.Seq[atuinResult]) (io.Reader, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	curDir, _ := os.Getwd() // best effort
	go func() {
		for r := range results {
			if r.Error != nil {
				// FIXME
				panic(err)
			}

			exitStatus := " "
			if r.Exit != "0" {
				exitStatus = tcolor.Red.Foreground("exit " + r.Exit)
			}

			dirCtx := ""
			if r.Directory == curDir {
				dirCtx = " \033[38;5;242m(current dir)\033[0m"
			}

			_, err := fmt.Fprintln(w, strings.Join([]string{
				r.Command,
				r.Exit,
				r.Directory,
				r.Duration,
				r.Time,
				exitStatus,
				dirCtx,
			}, _delim))
			if err != nil {
				// FIXME
				panic(err)
			}
		}

		if err := w.Close(); err != nil {
			// FIXME
			panic(err)
		}
	}()
	return r, nil
}

func fzf(input io.Reader, query string) error {
	selfExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("self executable: %w", err)
	}

	previewCmd := fmt.Sprintf("%s --preview {}", selfExe)
	fzfCmd := exec.Command(
		"fzf",
		"--tac",
		"--ansi",
		"--scheme", "history",
		"--prompt", "> ",
		"--header", "[Enter] to select, [Ctrl-Y] to yank.",
		"--preview", previewCmd,
		"--preview-window", "right:40%:wrap",
		"--delimiter", _delim,
		"--with-nth", "{1}  {6} {7}",
		"--accept-nth", "{1}",
		"--bind", "ctrl-y:execute-silent(echo -n {1} | pbcopy)+abort",
		"--query", query,
		"--height", "80%",
	)

	fzfCmd.Stdin = input
	fzfCmd.Stderr = os.Stderr
	fzfCmd.Stdout = os.Stdout

	if err := fzfCmd.Run(); err != nil {
		return fmt.Errorf("run fzf: %w", err)
	}

	return nil
}

func fzfPreview(data string) error {
	parts := strings.Split(data, _delim)
	if len(parts) < 5 {
		return fmt.Errorf("data format incorrect, expected 5 parts, got %d in %s", len(parts), data)
	}
	command, exitCode, directory, duration, timestamp := parts[0], parts[1], parts[2], parts[3], parts[4]

	exitCol := tcolor.Green
	if exitCode != "0" {
		exitCol = tcolor.Red
	}

	fmt.Println(tcolor.Bold("Full Command"))
	fmt.Println("───────────────────────────────────────────────────")
	fmt.Println(command)
	fmt.Println()
	fmt.Println(tcolor.Bold("Execution Details"))
	fmt.Println("───────────────────────────────────────────────────")
	fmt.Printf("%-10s %s\n", "Status:", exitCol.Foreground(exitCode))
	fmt.Printf("%-10s %s\n", "Ran In:", directory)
	fmt.Printf("%-10s %s\n", "Duration:", duration)
	fmt.Printf("%-10s %s\n", "When:", timestamp)
	fmt.Println()
	fmt.Println(tcolor.Bold("Recent Similar Commands"))
	fmt.Println("───────────────────────────────────────────────────")

	seen := make(map[atuinResult]bool)
	printResults := func(addArgs ...string) error {
		results, err := runAtuin(atuinParams{
			Query:          command,
			Limit:          5,
			AdditionalArgs: addArgs,
		})
		if err != nil {
			return err
		}

		for r := range results {
			if !seen[r] {
				seen[r] = true
				fmt.Printf("%-40.40s (%s)\n", r.Command, r.Directory)
			}
		}
		return nil
	}

	err := errors.Join(
		printResults(),
		printResults("--cwd", directory),
	)
	return err
}
