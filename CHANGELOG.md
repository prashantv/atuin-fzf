# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

<!--
## Template: Version - Date

summary if any

### Added

- details

### Removed

- details

### Fixed

- details
-->

## v0.0.3 - 2026-02-20

### Added

- Add support for search scopes (Ctrl-R) for only listing history within a directory.
- Support copying to clipboard on Windows via clip.exe.

### Fixed

- Hide the preview window if the terminal width is too small.
- Fix shell integration and fzf calls if binary location contains spaces.
- Improve error messages for unexpected arguments/data.
- Fix preview window missing and duplicate commands.

## v0.0.2 - 2025-11-13

### Fixed

- Fix handling of multi-line commands and display them as multi-line in fzf.
- Pass current buffer as initial search query.
- Avoid builder paths in stack traces.

## v0.0.1 - 2025-11-12

Initial test release of atuin-fzf.

See [README](https://github.com/bracesdev/errtrace#readme)
for more information.
