package main

import (
	"iter"
	"os"
	"strings"
)

type dirFilterMode string

const (
	dirFilterHost      dirFilterMode = "host"
	dirFilterDirectory dirFilterMode = "directory"
	dirFilterSubtree   dirFilterMode = "subtree"
	dirFilterWorkspace dirFilterMode = "workspace"
	dirFilterGlobal    dirFilterMode = "global"
)

var dirFilterCycle = []dirFilterMode{
	dirFilterHost,
	dirFilterDirectory,
	dirFilterSubtree,
	dirFilterWorkspace,
	dirFilterGlobal,
}

func nextDirFilter(current dirFilterMode) dirFilterMode {
	for i, m := range dirFilterCycle {
		if m == current {
			return dirFilterCycle[(i+1)%len(dirFilterCycle)]
		}
	}
	return dirFilterHost
}

func fetchFiltered(mode dirFilterMode, query string) (iter.Seq[atuinResult], error) {
	switch mode {
	case dirFilterDirectory, dirFilterWorkspace:
		// These map directly to atuin's built-in filter modes.
		return runAtuin(atuinParams{
			Query:      query,
			Limit:      1000,
			FilterMode: string(mode),
		})

	case dirFilterSubtree:
		// Fetch all results, then client-side filter to cwd subtree.
		results, err := fetchFiltered(dirFilterHost, query)
		if err != nil {
			return nil, err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return filterByDirPrefix(results, cwd), nil

	case dirFilterGlobal:
		global, err := runAtuin(atuinParams{Query: query, Limit: 1000, FilterMode: "global"})
		if err != nil {
			return nil, err
		}
		session, err := runAtuin(atuinParams{Query: query, Limit: 1000, FilterMode: "session"})
		if err != nil {
			return nil, err
		}
		return mergeRight(global, session), nil

	default: // dirFilterHost
		host, err := runAtuin(atuinParams{Query: query, Limit: 1000, FilterMode: "host"})
		if err != nil {
			return nil, err
		}
		session, err := runAtuin(atuinParams{Query: query, Limit: 1000, FilterMode: "session"})
		if err != nil {
			return nil, err
		}
		return mergeRight(host, session), nil
	}
}

func filterByDirPrefix(results iter.Seq[atuinResult], prefix string) iter.Seq[atuinResult] {
	return func(yield func(atuinResult) bool) {
		for r := range results {
			if r.Error != nil {
				yield(r)
				return
			}
			if r.Directory == prefix || strings.HasPrefix(r.Directory, prefix+"/") {
				if !yield(r) {
					return
				}
			}
		}
	}
}
