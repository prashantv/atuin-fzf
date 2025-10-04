package main

import (
	"bufio"
	"iter"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const _atuinDelim = "\t:::\t"

type atuinParams struct {
	Query          string
	Limit          int
	AdditionalArgs []string
}

type atuinResult struct {
	Time      string
	Duration  string
	Exit      string
	Directory string
	Command   string

	Error error
}

func runAtuin(p atuinParams) (iter.Seq[atuinResult], error) {
	format := strings.Join([]string{
		"{time}",
		"{duration}",
		"{exit}",
		"{directory}",
		"{command}", // intentionally last so command can contain the delimiter.
	}, _atuinDelim)

	args := []string{
		"search",
		"--limit", strconv.Itoa(p.Limit),
		"--format", format,
	}
	args = append(args, p.AdditionalArgs...)
	args = append(args, p.Query)

	cmd := exec.Command("atuin", args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return func(yield func(atuinResult) bool) {
		defer cmd.Wait()
		defer stdout.Close()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), _atuinDelim, 5)
			timestamp, duration, exitCode, directory, command := parts[0], parts[1], parts[2], parts[3], parts[4]

			if !yield(atuinResult{
				Time:      timestamp,
				Duration:  duration,
				Exit:      exitCode,
				Directory: directory,
				Command:   command,
			}) {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			yield(atuinResult{Error: err})
		}
	}, nil
}
