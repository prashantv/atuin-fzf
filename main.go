package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/prashantv/atuin-fzf/tcolor"
)

const _delim = "\t:::\t"

const (
	_unknownDir  = "unknown"
	_unknownCode = "-1"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [flags] [query]

Flags:
  --help          Show this help message
  --zsh           Print zsh shell integration
  --list          List history (without fzf)
  --preview       Show fzf preview (used internally)
  --clip          Copy stdin to clipboard (used internally)
  --fzf-actions   Generate fzf actions (used internally)
`, filepath.Base(os.Args[0]))
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h":
			usage()
			return nil
		case "--preview":
			if len(os.Args) < 3 {
				return fmt.Errorf("--preview requires an argument")
			}
			return fzfPreview(os.Args[2])
		case "--clip":
			return clip()
		case "--zsh":
			exe, err := os.Executable()
			if err != nil {
				exe = os.Args[0]
			}
			fmt.Printf(_zshFn, exe)
			return nil
		case "--list":
			mode, query := parseListArgs(os.Args[2:])
			return list(mode, query)
		case "--fzf-actions":
			if len(os.Args) < 3 {
				return fmt.Errorf("--fzf-actions requires current prompt argument")
			}
			fzfActions(os.Args[2])
			return nil
		default:
			if strings.HasPrefix(os.Args[1], "-") {
				return fmt.Errorf("unknown flag: %s\nRun '%s --help' for usage", os.Args[1], filepath.Base(os.Args[0]))
			}
		}
	}

	var initialQuery string
	if len(os.Args) > 1 {
		initialQuery = os.Args[1]
	}

	return runInteractive(initialQuery)
}

func runInteractive(query string) error {
	results, err := fetchFiltered(dirFilterAll, query)
	if err != nil {
		return err
	}

	fzfInput := atuinToFzf(results)

	if err := fzf(fzfInput, query); err != nil {
		return err
	}

	return nil
}

func parseListArgs(args []string) (dirFilterMode, string) {
	mode := dirFilterAll
	var queryParts []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir-filter" && i+1 < len(args) {
			mode = dirFilterMode(args[i+1])
			i++
		} else if v, ok := strings.CutPrefix(args[i], "--dir-filter="); ok {
			mode = dirFilterMode(v)
		} else {
			queryParts = append(queryParts, args[i])
		}
	}
	return mode, strings.Join(queryParts, " ")
}

func list(mode dirFilterMode, query string) error {
	results, err := fetchFiltered(mode, query)
	if err != nil {
		return err
	}

	fzfInput := atuinToFzf(results)

	_, err = io.Copy(os.Stdout, fzfInput)
	return err
}

func fzfActions(currentPrompt string) {
	// Extract mode from prompt like "all> " or "directory> "
	current := dirFilterMode(strings.TrimSuffix(currentPrompt, "> "))
	next := nextDirFilter(current)

	selfExe, err := os.Executable()
	if err != nil {
		selfExe = os.Args[0]
	}

	fmt.Printf("reload(%s --list --dir-filter=%s {q})+change-prompt(%s> )", selfExe, next, next)
}

func atuinToFzf(results iter.Seq[atuinResult]) io.Reader {
	r, w := io.Pipe()

	curDir, _ := os.Getwd() // best effort
	go func() {
		for r := range results {
			if r.Error != nil {
				w.CloseWithError(r.Error)
				return
			}

			dirCtx := ""
			if r.Directory == curDir {
				dirCtx = tcolor.Gray.Foreground("(same cwd)")
			}

			_, err := fmt.Fprint(w, strings.Join([]string{
				r.Command,
				r.Exit,
				r.Directory,
				r.Duration,
				r.Time,
				r.RelativeTime,
				exitColor(r.Exit),
				dirCtx,
				string(byte(0)),
			}, _delim))
			if err != nil {
				w.CloseWithError(err)
				return
			}
		}

		w.Close()
	}()
	return r
}

func fzf(input io.Reader, query string) error {
	selfExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("self executable: %w", err)
	}

	previewCmd := fmt.Sprintf("%s --preview {}", selfExe)
	fzfCmd := exec.Command(
		"fzf",
		"--read0",
		"--tac",
		"--ansi",
		"--scheme", "history",
		"--prompt", "all> ",
		"--header", "[Enter] select  [Ctrl-O] cd & use  [Ctrl-Y] yank  [Ctrl-R] dir filter",
		"--preview", previewCmd,
		"--preview-window", "right:40%:wrap,<50(hidden)",
		"--delimiter", _delim,
		"--with-nth", "{1}  {7} {8}",
		"--accept-nth", "{1}",
		"--bind", fmt.Sprintf("ctrl-y:execute-silent(echo -n {1} | %s --clip)+abort", selfExe),
		"--bind", "ctrl-o:become(printf \"CHDIR:\\t%s\\t%s\" {3} {1})",
		"--bind", fmt.Sprintf("ctrl-r:transform(%s --fzf-actions {fzf:prompt})", selfExe),
		"--query", query,
		"--height", "80%",
	)

	fzfCmd.Stdin = input
	fzfCmd.Stderr = os.Stderr
	fzfCmd.Stdout = os.Stdout

	if err := fzfCmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok && err.ExitCode() == 130 {
			// User-interrupted.
			return nil
		}

		return fmt.Errorf("run fzf: %w", err)
	}

	return nil
}

func fzfPreview(data string) error {
	const expectedParts = 6
	parts := strings.Split(data, _delim)
	if len(parts) < expectedParts {
		return fmt.Errorf("fzf preview input has fewer parts (%d) than expected (%d): %q", len(parts), expectedParts, data)
	}
	command, exitCode, cwd, duration, timestamp, relTimestamp := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]

	exitCol := tcolor.Green
	if exitCode != "0" {
		exitCol = tcolor.Red
	}
	if exitCode == _unknownCode {
		exitCode = "unknown"
		exitCol = tcolor.Gray
	}
	cwdDisplay := cwd
	if cwd == _unknownDir {
		cwdDisplay = tcolor.Gray.Foreground(cwd)
	} else {
		cwdDisplay = shortenHome(cwd)
	}

	fmt.Println(tcolor.Bold("Command"))
	fmt.Println("────────────────────────")
	fmt.Println(command)
	fmt.Println()
	fmt.Println(tcolor.Bold("Execution Details"))
	fmt.Println("────────────────────────")
	fmt.Printf("%-10s %s %s\n", "When:", timestamp, tcolor.Cyan.Foreground(relTimestamp+" ago"))
	fmt.Printf("%-10s %s\n", "Directory:", cwdDisplay)
	fmt.Printf("%-10s %s\n", "Exit Code:", exitCol.Foreground(exitCode))
	fmt.Printf("%-10s %s\n", "Duration:", duration)
	fmt.Println()
	fmt.Println(tcolor.Bold("Recent Similar Commands"))
	fmt.Println("────────────────────────")

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
			dirDisplay := shortenHome(r.Directory)
			if r.Directory == cwd {
				dirDisplay = "(same cwd)"
			}

			cmpR := r
			cmpR.RelativeTime = ""

			if !seen[cmpR] {
				seen[cmpR] = true
				fmt.Printf("%s %s %s\n%s\n",
					tcolor.Cyan.Foreground(r.RelativeTime),
					tcolor.Gray.Foreground(dirDisplay),
					exitColor(r.Exit),
					tcolor.Bold("$ ")+r.Command,
				)
			}
		}
		return nil
	}

	err := errors.Join(
		printResults(),
		printResults("--cwd", cwd),
	)
	return err
}

func exitColor(exitCode string) string {
	if exitCode == _unknownCode {
		return ""
	}
	if exitCode != "0" {
		return tcolor.Red.Foreground("exit " + exitCode)
	}
	return ""
}

func shortenHome(s string) string {
	if s == _unknownDir {
		return ""
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		if suffix, ok := strings.CutPrefix(s, homeDir); ok {
			return filepath.Join("~", suffix)
		}
	}

	return s
}

func clip() error {
	clipCmd := os.Getenv("ATUIN_CLIP")
	if clipCmd == "" {
		for _, candidate := range []string{"pbcopy", "clip.exe"} {
			if _, err := exec.LookPath(candidate); err == nil {
				clipCmd = candidate
				break
			}
		}
	}
	if clipCmd == "" {
		return fmt.Errorf("no clipboard command found: set ATUIN_CLIP or install pbcopy/clip.exe")
	}

	var stdin io.Reader = os.Stdin
	// clip.exe interprets piped input as UTF-16LE, so convert from UTF-8.
	if filepath.Base(clipCmd) == "clip.exe" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		stdin = utf8ToUTF16LE(input)
	}

	cmd := exec.Command(clipCmd)
	cmd.Stdin = stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// utf8ToUTF16LE converts UTF-8 bytes to a UTF-16LE reader with a BOM.
func utf8ToUTF16LE(b []byte) io.Reader {
	encoded := utf16.Encode([]rune(string(b)))
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFE}) // UTF-16LE BOM
	binary.Write(&buf, binary.LittleEndian, encoded)
	return &buf
}

// mergeRight merges results sequences, preferring results on the right.
func mergeRight(res1, res2 iter.Seq[atuinResult]) iter.Seq[atuinResult] {
	var res2Vals []atuinResult
	seen := make(map[atuinResult]struct{})
	for r := range res2 {
		seen[r] = struct{}{}
		res2Vals = append(res2Vals, r)
	}

	return func(yield func(atuinResult) bool) {
		for r := range res1 {
			if _, ok := seen[r]; ok {
				continue
			}

			if !yield(r) {
				return
			}
		}

		for _, r := range res2Vals {
			if !yield(r) {
				return
			}
		}
	}
}
