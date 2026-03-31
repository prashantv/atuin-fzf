package main

import (
	"bytes"
	"encoding/base64"
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
	"unicode"
	"unicode/utf16"

	"github.com/prashantv/atuin-fzf/tcolor"
)

const _delim = " \u200B"

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
  --fzf-reload    Generate fzf reload action (used internally)
  --fzf-filter    Toggle fzf / atuin filtering mode (used internally)
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
			fmt.Printf(_zshFn, selfExePath())
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
		case "--fzf-reload":
			if len(os.Args) < 3 {
				return fmt.Errorf("--fzf-reload requires current prompt argument")
			}
			fzfReload(os.Args[2])
			return nil
		case "--fzf-filter":
			if len(os.Args) < 3 {
				return fmt.Errorf("--fzf-filter requires current prompt argument")
			}
			fzfToggle(os.Args[2])
			return nil
		default:
			if strings.HasPrefix(os.Args[1], "-") {
				return fmt.Errorf("unknown flag: %s\nRun '%s --help' for usage", os.Args[1], filepath.Base(os.Args[0]))
			}
		}
	}

	initialQuery := strings.Join(os.Args[1:], " ")
	return runInteractive(initialQuery)
}

func runInteractive(query string) error {
	results, err := fetchFiltered(dirFilterHost, query)
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
	mode := dirFilterHost
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

func selfExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	return exe
}

// parseFzfPrompt extracts the filter mode and filter type from an fzf prompt.
// e.g. "host> " → (host, queryFilterFzf), "host| " → (host, queryFilterAtuin)
func parseFzfPrompt(prompt string) (dirFilterMode, queryFilterMode) {
	i := strings.IndexFunc(prompt, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	if i < 0 {
		return dirFilterMode(prompt), queryFilterFzf
	}

	mode := dirFilterMode(prompt[:i])
	sep := string(prompt[i])
	for _, qf := range []queryFilterMode{queryFilterAtuin, queryFilterFzf} {
		if sep == qf.promptChar() {
			return mode, qf
		}
	}
	return mode, queryFilterFzf
}

func buildFzfPrompt(mode dirFilterMode, qf queryFilterMode) string {
	return string(mode) + qf.promptChar() + " "
}

func fzfActions(currentPrompt string) {
	current, qf := parseFzfPrompt(currentPrompt)
	next := nextDirFilter(current)
	fmt.Print(reloadWith(next, qf))
}

func fzfReload(currentPrompt string) {
	current, _ := parseFzfPrompt(currentPrompt)
	fmt.Print(reloadAction(current, queryFilterAtuin))
}

func fzfToggle(currentPrompt string) {
	current, qf := parseFzfPrompt(currentPrompt)
	fmt.Print(reloadWith(current, qf.toggle()))
}

func reloadWith(mode dirFilterMode, qf queryFilterMode) string {
	return strings.Join([]string{
		qf.changeBind() + "(change)",
		reloadAction(mode, qf),
		fmt.Sprintf("change-prompt(%s)", buildFzfPrompt(mode, qf)),
		fmt.Sprintf("change-header(%s)", fzfHeader(qf)),
	}, "+")
}

func reloadAction(mode dirFilterMode, qf queryFilterMode) string {
	selfExe := selfExePath()
	return fmt.Sprintf("reload(%q --list --dir-filter=%s%s)", selfExe, mode, qf.atuinFilter())
}

func atuinToFzf(results iter.Seq[atuinResult]) io.Reader {
	r, w := io.Pipe()

	curDir, _ := os.Getwd()     // best effort
	curHost, _ := os.Hostname() // best effort

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

			hostCtx := ""
			if curHost != "" && r.Host != curHost {
				hostCtx = tcolor.Blue.Foreground(r.Host)
			}

			_, err := fmt.Fprint(w, strings.Join([]string{
				r.Command,
				r.Exit,
				r.Directory,
				r.Duration,
				r.Time,
				r.RelativeTime,
				r.Host,
				exitColor(r.Exit),
				dirCtx,
				hostCtx,
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

func fzfHeader(qf queryFilterMode) string {
	return fmt.Sprintf("[Enter] select  [Ctrl-O] cd & use  [Ctrl-Y] yank  [Ctrl-R] filter  [Ctrl-F] %s", qf.promptLabel())
}

func fzf(input io.Reader, query string) error {
	selfExe := selfExePath()

	previewCmd := fmt.Sprintf("%q --preview {}", selfExe)
	args := []string{
		"--read0",
		"--tac",
		"--ansi",
		"--scheme", "history",
		"--prompt", buildFzfPrompt(dirFilterHost, queryFilterFzf),
		"--header", fzfHeader(queryFilterFzf),
		"--preview", previewCmd,
		"--preview-window", "right:40%:wrap,<50(hidden)",
		"--delimiter", _delim,
		"--nth", "1",
		"--with-nth", "{1}" + _delim + " {8}" + _delim + "{9}" + _delim + "{10}",
		"--accept-nth", "{1}",
		"--bind", fmt.Sprintf("ctrl-y:execute-silent(echo -n {1} | %q --clip)+abort", selfExe),
		"--bind", "ctrl-o:become(printf \"CHDIR:\\t%s\\t%s\" {3} {1})",
		"--bind", fmt.Sprintf("ctrl-r:transform(%q --fzf-actions {fzf:prompt})", selfExe),
		"--bind", fmt.Sprintf("ctrl-f:transform(%q --fzf-filter {fzf:prompt})", selfExe),

		// When filtering with atuin, we need a change:transform.
		"--bind", fmt.Sprintf("change:transform(%q --fzf-reload {fzf:prompt})", selfExe),
		// but start in fzf filtering mode.
		"--bind", "start:unbind(change)",
		"--query", query,
		"--height", "80%",
	}
	fzfCmd := exec.Command("fzf", args...)

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
	const expectedParts = 7
	parts := strings.Split(data, _delim)
	if len(parts) < expectedParts {
		return fmt.Errorf("fzf preview input has fewer parts (%d) than expected (%d): %q", len(parts), expectedParts, data)
	}
	command, exitCode, cwd, duration, timestamp, relTimestamp, host := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]

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
	fmt.Printf("%-10s %s\n", "Host:", host)
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
			if r.Error != nil {
				return err
			}

			dirDisplay := shortenHome(r.Directory)
			if r.Directory == cwd {
				dirDisplay = "(same cwd)"
			}

			cmpR := r
			cmpR.RelativeTime = ""

			if !seen[cmpR] {
				seen[cmpR] = true
				fmt.Printf("%s %s %s %s\n%s\n",
					tcolor.Blue.Foreground(r.Host),
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
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	// If ATUIN_CLIP is set, use the specified command.
	if clipCmd := os.Getenv("ATUIN_CLIP"); clipCmd != "" {
		var stdin io.Reader = strings.NewReader(string(input))
		// clip.exe interprets piped input as UTF-16LE, so convert from UTF-8.
		if filepath.Base(clipCmd) == "clip.exe" {
			stdin = utf8ToUTF16LE(input)
		}
		cmd := exec.Command(clipCmd)
		cmd.Stdin = stdin
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Use OSC52 escape sequence to set the terminal clipboard.
	// Write to /dev/tty to ensure we reach the terminal directly,
	// since stdout/stderr may be piped by fzf.
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open /dev/tty: %w", err)
	}
	defer tty.Close()

	encoded := base64.StdEncoding.EncodeToString(input)
	_, err = fmt.Fprintf(tty, "\033]52;c;%s\a", encoded)
	return err
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
